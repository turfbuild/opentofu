// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plans

import (
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
)

// ActionInvocationInstanceSrc describes a single planned invocation of a
// provider action (the Terraform 1.14 actions feature): one action instance,
// invoked once, either from a resource's lifecycle (an action_trigger) or
// imperatively (-invoke). It is the plan-level counterpart of the
// configuration-level Action/ActionTrigger blocks parsed in package configs,
// and it carries what the plan document's action_invocations[] entry prints.
//
// OpenTofu does not yet implement actions natively (tracking
// opentofu/opentofu#3309). This is an additive, downstream-maintained
// extension so that a plan can carry action invocations alongside its
// resource and output changes. It is in-memory only: there is intentionally no
// planproto serialization, because no caller persists a plan file with
// invocations. When upstream lands actions, this type collapses onto the
// upstream representation.
type ActionInvocationInstanceSrc struct {
	// Addr is the action instance's absolute address: action.<type>.<name>,
	// prefixed by its module instance path and suffixed by its instance key
	// when it has them.
	Addr string

	// Type is the action type, and Name the action block's name label.
	Type string
	Name string

	// Provider is the provider that serves the action type.
	Provider addrs.Provider

	// Config is the action's configuration as the plan evaluated it: unknown
	// where the plan could not know a value, carrying the sensitive marks it
	// was evaluated with. cty.NilVal or a null value for a block with no
	// configuration.
	Config cty.Value

	// Exactly one of the two triggers is set.
	LifecycleTrigger *ActionLifecycleTrigger
	InvokeTrigger    *ActionInvokeTrigger
}

// ActionLifecycleTrigger is the resource lifecycle edge that invokes an action:
// the triggering resource instance, the event, and where the invocation came
// from in the trigger blocks — the coordinates that tell two invocations of
// one action on one resource apart.
type ActionLifecycleTrigger struct {
	TriggeringResourceAddr string

	// Event is the lifecycle edge as written in `events`: before_create …
	// after_destroy.
	Event string

	// BlockIndex is the trigger block's position among the triggering
	// resource's action_trigger blocks, and ListIndex the action's position in
	// that block's `actions` list.
	BlockIndex int
	ListIndex  int

	// OnFailure is the trigger's on_failure as written: halt, continue, or
	// taint. "" is halt.
	OnFailure string
}

// ActionInvokeTrigger marks an imperative invocation (-invoke).
// CallingResourceAddr is the resource instance whose configuration the
// action's configuration read as `caller`, when it read one.
type ActionInvokeTrigger struct {
	CallingResourceAddr string
}
