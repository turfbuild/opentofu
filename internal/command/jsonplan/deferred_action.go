// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsonplan

import (
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
