// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

// Package jsonstate is a stable boundary over OpenTofu's internal
// command/jsonstate package — the canonical `tofu show -json` state
// marshaller, and the format of a plan document's prior_state. Inputs are the
// aliased x types: *statefile.File and *tofu.Schemas.
package jsonstate

import (
	"github.com/opentofu/opentofu/internal/command/jsonstate"
)

// Marshal returns the standard json-format encoding of a statefile.
var Marshal = jsonstate.Marshal

// MarshalForLog returns the same document as Marshal, as the structured State
// rather than its encoding.
var MarshalForLog = jsonstate.MarshalForLog

// SensitiveAsBool projects a mark-carrying value into the document's
// sensitive_values shape, as a cty.Value: true at every sensitive path, the
// whole subtree beneath redacted, non-sensitive entries omitted.
var SensitiveAsBool = jsonstate.SensitiveAsBool

// FormatVersion is the json-format version Marshal emits.
const FormatVersion = jsonstate.FormatVersion

// The document's types, from the top.
type (
	State           = jsonstate.State
	StateValues     = jsonstate.StateValues
	Module          = jsonstate.Module
	Resource        = jsonstate.Resource
	AttributeValues = jsonstate.AttributeValues
	Output          = jsonstate.Output
)
