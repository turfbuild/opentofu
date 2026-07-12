// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

// Package jsonplan is a stable boundary over OpenTofu's internal
// command/jsonplan package — the canonical `tofu show -json` marshaller. A consumer
// renders a phase's plan by handing the plan, its config, prior state, and the
// aggregate schemas to Marshal, producing standard json-format output
// (tfplan/v2 + tfstate/v2 via prior_state + tfconfig/v2 via configuration) that
// tools like HashiCorp Sentinel consume directly.
package jsonplan

import (
	"github.com/opentofu/opentofu/internal/command/jsonplan"
)

// Marshal returns the standard json-format encoding of a plan. Inputs are the
// aliased x types: *configs.Config, *plans.Plan, *statefile.File,
// *tofu.Schemas.
var Marshal = jsonplan.Marshal

// MarshalForRenderer returns the subset used by the structured renderer.
var MarshalForRenderer = jsonplan.MarshalForRenderer

// Output types, re-exported for callers that post-process the marshalled plan.
type (
	Plan                   = jsonplan.Plan
	ResourceChange         = jsonplan.ResourceChange
	Change                 = jsonplan.Change
	DeferredResourceChange = jsonplan.DeferredResourceChange
	ActionInvocation       = jsonplan.ActionInvocation
)

// FormatVersion is the json-format version Marshal emits.
const FormatVersion = jsonplan.FormatVersion
