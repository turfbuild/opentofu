// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsonplan

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/opentofu/opentofu/internal/command/jsonstate"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/plans"
)

// ActionInvocation is one action_invocations[] entry, in the shape Terraform's
// plan document gives it (Terraform 1.14+ actions; the JSON format page does
// not document the section, so the shape is the one `terraform show -json`
// prints). config_values carries the known parts of the evaluated
// configuration, and config_unknown and config_sensitive the boolean trees
// marking its unknown and its sensitive leaves -- a resource change's
// after/after_unknown/after_sensitive conventions applied to a config, the
// values printed in the clear beside them. Exactly one of the two triggers is
// present. Downstream extension (OpenTofu tracks actions in
// opentofu/opentofu#3309).
type ActionInvocation struct {
	Address                string                  `json:"address"`
	Type                   string                  `json:"type"`
	Name                   string                  `json:"name"`
	ConfigValues           json.RawMessage         `json:"config_values"`
	ConfigSensitive        json.RawMessage         `json:"config_sensitive"`
	ConfigUnknown          json.RawMessage         `json:"config_unknown"`
	ProviderName           string                  `json:"provider_name"`
	LifecycleActionTrigger *LifecycleActionTrigger `json:"lifecycle_action_trigger,omitempty"`
	InvokeActionTrigger    *InvokeActionTrigger    `json:"invoke_action_trigger,omitempty"`
}

// LifecycleActionTrigger is the trigger that produced a lifecycle invocation,
// in Terraform's spellings: the event camel-cased ("AfterCreate"), on_failure
// as "ActionOnFailureHalt" / "…Continue" / "…Taint", and the block and list
// coordinates that tell two invocations of one action apart.
type LifecycleActionTrigger struct {
	TriggeringResourceAddress string `json:"triggering_resource_address"`
	ActionTriggerEvent        string `json:"action_trigger_event"`
	ActionTriggerBlockIndex   int    `json:"action_trigger_block_index"`
	ActionsListIndex          int    `json:"actions_list_index"`
	OnFailure                 string `json:"on_failure"`
}

// InvokeActionTrigger marks an imperative invocation: `{}`, or the resource
// instance the action's configuration read as `caller`.
type InvokeActionTrigger struct {
	CallingResourceAddress string `json:"calling_resource_address,omitempty"`
}

// MarshalActionInvocations projects a plan's planned action invocations into
// action_invocations[] entries, listed in Terraform's order: lifecycle
// invocations by triggering resource, then the event's place in the lifecycle,
// then the trigger block and the action's place in its list; imperative ones
// by action address, then calling resource. The carrier holds exactly the
// invocations the plan will run, so every one is printed.
func MarshalActionInvocations(invocations []*plans.ActionInvocationInstanceSrc) ([]ActionInvocation, error) {
	if len(invocations) == 0 {
		return nil, nil
	}
	out := make([]ActionInvocation, 0, len(invocations))
	for _, ai := range invocations {
		values, unknown, sensitive, err := marshalActionConfig(ai.Config)
		if err != nil {
			return nil, err
		}
		inv := ActionInvocation{
			Address:         ai.Addr,
			Type:            ai.Type,
			Name:            ai.Name,
			ConfigValues:    values,
			ConfigSensitive: sensitive,
			ConfigUnknown:   unknown,
		}
		if !ai.Provider.IsZero() {
			inv.ProviderName = ai.Provider.String()
		}
		switch {
		case ai.LifecycleTrigger != nil:
			t := ai.LifecycleTrigger
			inv.LifecycleActionTrigger = &LifecycleActionTrigger{
				TriggeringResourceAddress: t.TriggeringResourceAddr,
				ActionTriggerEvent:        actionEventJSONName(t.Event),
				ActionTriggerBlockIndex:   t.BlockIndex,
				ActionsListIndex:          t.ListIndex,
				OnFailure:                 actionOnFailureJSONName(t.OnFailure),
			}
		default:
			inv.InvokeActionTrigger = &InvokeActionTrigger{}
			if ai.InvokeTrigger != nil {
				inv.InvokeActionTrigger.CallingResourceAddress = ai.InvokeTrigger.CallingResourceAddr
			}
		}
		out = append(out, inv)
	}
	sort.SliceStable(out, func(i, j int) bool {
		x, y := out[i], out[j]
		a, b := x.LifecycleActionTrigger, y.LifecycleActionTrigger
		switch {
		case a == nil && b == nil:
			if x.Address != y.Address {
				return x.Address < y.Address
			}
			return x.InvokeActionTrigger.CallingResourceAddress < y.InvokeActionTrigger.CallingResourceAddress
		case a == nil || b == nil:
			// One plan holds one kind; should two meet, lifecycle first.
			return a != nil
		}
		if a.TriggeringResourceAddress != b.TriggeringResourceAddress {
			return a.TriggeringResourceAddress < b.TriggeringResourceAddress
		}
		if ea, eb := actionEventRank(a.ActionTriggerEvent), actionEventRank(b.ActionTriggerEvent); ea != eb {
			return ea < eb
		}
		if a.ActionTriggerBlockIndex != b.ActionTriggerBlockIndex {
			return a.ActionTriggerBlockIndex < b.ActionTriggerBlockIndex
		}
		return a.ActionsListIndex < b.ActionsListIndex
	})
	return out, nil
}

// marshalActionConfig renders an invocation's configuration as the plan
// document does: config_values with the unknown leaves omitted,
// config_unknown the tree marking them, config_sensitive the tree marking the
// sensitive leaves. No configuration renders as null, {}, {}. Both trees are
// always built: Terraform prints a known list as one false per element in
// config_unknown, where a resource change's after_unknown is the empty object
// for a wholly known value.
func marshalActionConfig(val cty.Value) (values, unknown, sensitive json.RawMessage, err error) {
	values, unknown, sensitive = json.RawMessage("null"), json.RawMessage("{}"), json.RawMessage("{}")
	if val == cty.NilVal || val.IsNull() {
		return values, unknown, sensitive, nil
	}
	unmarked, pvms := val.UnmarkDeepWithPaths()
	// Only the sensitive marks are projected: the projection iterates what it
	// is given, and a collection carrying any other mark cannot be iterated.
	var sensitivePaths []cty.PathValueMarks
	for _, pvm := range pvms {
		if _, ok := pvm.Marks[marks.Sensitive]; ok {
			sensitivePaths = append(sensitivePaths, cty.PathValueMarks{Path: pvm.Path, Marks: cty.NewValueMarks(marks.Sensitive)})
		}
	}
	tree := jsonstate.SensitiveAsBool(unmarked.MarkWithPaths(sensitivePaths))
	if sensitive, err = ctyjson.Marshal(tree, tree.Type()); err != nil {
		return nil, nil, nil, err
	}
	known := unmarked
	if !unmarked.IsWhollyKnown() {
		known = omitUnknowns(unmarked)
	}
	if known != cty.NilVal && !known.IsNull() {
		if values, err = ctyjson.Marshal(known, known.Type()); err != nil {
			return nil, nil, nil, err
		}
	}
	u := unknownAsBool(unmarked)
	if unknown, err = ctyjson.Marshal(u, u.Type()); err != nil {
		return nil, nil, nil, err
	}
	return values, unknown, sensitive, nil
}

// actionEventJSONName camel-cases a lifecycle event: after_create →
// AfterCreate.
func actionEventJSONName(event string) string {
	var b strings.Builder
	for _, part := range strings.Split(event, "_") {
		if part == "" {
			continue
		}
		b.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}
	return b.String()
}

// actionEventRank is an event's place in the lifecycle, for the listing
// order.
func actionEventRank(camel string) int {
	switch camel {
	case "BeforeCreate":
		return 0
	case "AfterCreate":
		return 1
	case "BeforeUpdate":
		return 2
	case "AfterUpdate":
		return 3
	case "BeforeDestroy":
		return 4
	case "AfterDestroy":
		return 5
	}
	return 6
}

// actionOnFailureJSONName spells on_failure as the plan document does:
// halt → ActionOnFailureHalt; unset is halt.
func actionOnFailureJSONName(onFailure string) string {
	if onFailure == "" {
		onFailure = "halt"
	}
	return "ActionOnFailure" + strings.ToUpper(onFailure[:1]) + onFailure[1:]
}
