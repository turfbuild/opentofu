// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsonplan

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/tofu"
)

// DeferredResourceChange mirrors Terraform 1.9+ jsonplan deferred_changes[]:
// a resource_change the provider could not fully plan this round, wrapped with
// the protocol-level reason it was deferred. Downstream extension — OpenTofu
// removed deferred actions (DeferralAllowed) in 1.11.
type DeferredResourceChange struct {
	ResourceChange ResourceChange `json:"resource_change"`
	Reason         string         `json:"reason"`
}

// ActionInvocation mirrors a Terraform 1.14 jsonplan action_invocations[] entry:
// the action address/type/name, its provider, the (pre-resolution) config, and —
// for a lifecycle-bound invocation — the triggering resource and event. Standalone
// invocations omit the trigger fields. Downstream extension (OpenTofu tracks
// actions in opentofu/opentofu#3309).
type ActionInvocation struct {
	Address                   string          `json:"address"`
	Type                      string          `json:"type"`
	Name                      string          `json:"name"`
	ProviderName              string          `json:"provider_name,omitempty"`
	Config                    json.RawMessage `json:"config,omitempty"`
	TriggeringResourceAddress string          `json:"triggering_resource_address,omitempty"`
	TriggeringEvent           string          `json:"triggering_event,omitempty"`
}

// nonDeferredChanges returns the subset of changes that are not deferred, in the
// original order. resource_changes and planned_values are built from this subset;
// deferred entries surface only in deferred_changes.
func nonDeferredChanges(resources []*plans.ResourceInstanceChangeSrc) []*plans.ResourceInstanceChangeSrc {
	var out []*plans.ResourceInstanceChangeSrc
	for _, rc := range resources {
		if rc.DeferredReason != "" {
			continue
		}
		out = append(out, rc)
	}
	return out
}

// MarshalDeferredChanges projects the deferred subset of a plan's resource
// changes into deferred_changes[] entries. Each deferred change is marshalled
// through MarshalResourceChanges (so its resource_change is byte-identical to a
// normal one) and paired with its DeferredReason.
func MarshalDeferredChanges(resources []*plans.ResourceInstanceChangeSrc, schemas *tofu.Schemas) ([]DeferredResourceChange, error) {
	var deferred []*plans.ResourceInstanceChangeSrc
	reasonByKey := make(map[string]string)
	for _, rc := range resources {
		if rc.DeferredReason == "" {
			continue
		}
		deferred = append(deferred, rc)
		reasonByKey[rc.Addr.String()+"\x00"+rc.DeposedKey.String()] = rc.DeferredReason
	}
	if len(deferred) == 0 {
		return nil, nil
	}
	rcs, err := MarshalResourceChanges(deferred, schemas)
	if err != nil {
		return nil, err
	}
	out := make([]DeferredResourceChange, 0, len(rcs))
	for _, r := range rcs {
		out = append(out, DeferredResourceChange{
			ResourceChange: r,
			Reason:         reasonByKey[r.Address+"\x00"+r.Deposed],
		})
	}
	return out, nil
}

// MarshalActionInvocations projects a plan's planned provider-action invocations
// into action_invocations[] entries. resources is the plan's resource changes,
// used to apply the op-availability filter: a lifecycle-bound invocation is
// emitted only if its triggering resource's planned action includes the event's
// operation (a create plan does not advertise a before_destroy trigger, nor a
// destroy plan a before_create) — matching Terraform's jsonplan, which carries
// only the firing set. Standalone invocations (no triggering resource) always fire.
func MarshalActionInvocations(invocations []*plans.ActionInvocationInstanceSrc, resources []*plans.ResourceInstanceChangeSrc) ([]ActionInvocation, error) {
	if len(invocations) == 0 {
		return nil, nil
	}
	actionByAddr := make(map[string]plans.Action, len(resources))
	for _, rc := range resources {
		if rc != nil {
			actionByAddr[rc.Addr.String()] = rc.Action
		}
	}
	out := make([]ActionInvocation, 0, len(invocations))
	for _, ai := range invocations {
		if ai.TriggeringResourceAddr != "" {
			act, planned := actionByAddr[ai.TriggeringResourceAddr]
			if !planned || !actionInvocationFires(ai.TriggerEvent, act) {
				continue
			}
		}
		inv := ActionInvocation{
			Address:                   ai.Addr,
			Type:                      ai.Type,
			Name:                      actionNameFromAddr(ai.Addr),
			ProviderName:              ai.ProviderNamespace + "/" + ai.ProviderName,
			TriggeringResourceAddress: ai.TriggeringResourceAddr,
			TriggeringEvent:           ai.TriggerEvent,
		}
		if len(ai.Config) > 0 {
			raw, err := json.Marshal(ai.Config)
			if err != nil {
				return nil, err
			}
			inv.Config = raw
		}
		out = append(out, inv)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Address < out[j].Address })
	return out, nil
}

// actionInvocationFires reports whether a triggering event runs given the
// triggering resource's planned action: a create-side event needs a create, a
// destroy-side event a delete/forget, an update event an update. Replace (either
// ordering) carries both a create and a destroy, so it satisfies create- and
// destroy-side events alike.
func actionInvocationFires(event string, action plans.Action) bool {
	switch event {
	case "before_create", "after_create":
		switch action {
		case plans.Create, plans.CreateThenDelete, plans.DeleteThenCreate:
			return true
		}
	case "before_update", "after_update":
		return action == plans.Update
	case "before_destroy", "after_destroy":
		switch action {
		case plans.Delete, plans.Forget, plans.CreateThenDelete, plans.DeleteThenCreate:
			return true
		}
	}
	return false
}

// actionNameFromAddr returns the <name> segment of an action.<type>.<name>
// address (action types and names are identifiers, never dotted).
func actionNameFromAddr(addr string) string {
	parts := strings.SplitN(addr, ".", 3)
	if len(parts) == 3 {
		return parts[2]
	}
	return addr
}
