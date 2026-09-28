// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

// Package jsonplan is a stable boundary over OpenTofu's internal
// command/jsonplan package — the canonical `tofu show -json` marshaller. A consumer
// renders a phase's plan by handing the plan, its config, prior state, and the
// aggregate schemas to Marshal, producing standard json-format output
// (tfplan/v2 + tfstate/v2 via prior_state + tfconfig/v2 via configuration) that
// tools like HashiCorp Sentinel consume directly.
//
// Inputs are the aliased x types: *configs.Config, *plans.Plan,
// *statefile.File, *tofu.Schemas. The output types are re-exported whole, so
// a consumer that wants to post-process the document — or compare its own
// export against this one — can hold the structured form MarshalForLog
// returns rather than re-parsing bytes.
package jsonplan

import (
	"github.com/opentofu/opentofu/internal/command/jsonplan"
)

// Marshal returns the standard json-format encoding of a plan.
var Marshal = jsonplan.Marshal

// MarshalForLog returns the same document as Marshal, as the structured Plan
// rather than its encoding.
var MarshalForLog = jsonplan.MarshalForLog

// FormatVersion is the json-format version Marshal emits.
const FormatVersion = jsonplan.FormatVersion

// The document's types, from the top: Plan holds the planned values, the
// resource, output, deferred and action changes, the variables, and the prior
// state and configuration.
type (
	Plan                   = jsonplan.Plan
	Variables              = jsonplan.Variables
	Variable               = jsonplan.Variable
	StateValues            = jsonplan.StateValues
	Module                 = jsonplan.Module
	Resource               = jsonplan.Resource
	AttributeValues        = jsonplan.AttributeValues
	Output                 = jsonplan.Output
	ResourceChange         = jsonplan.ResourceChange
	Change                 = jsonplan.Change
	Importing              = jsonplan.Importing
	ResourceAttr           = jsonplan.ResourceAttr
	DeferredResourceChange = jsonplan.DeferredResourceChange
	ActionInvocation       = jsonplan.ActionInvocation
	LifecycleActionTrigger = jsonplan.LifecycleActionTrigger
	InvokeActionTrigger    = jsonplan.InvokeActionTrigger
	TurfOverlay            = jsonplan.TurfOverlay
)
