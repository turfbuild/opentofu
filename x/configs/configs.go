// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

// Package configs re-exports OpenTofu's internal configuration types through a stable API boundary.
// This enables configuration parsing and analysis without direct imports of OpenTofu internals.
// Addresses — module, provider, reference subjects, resource modes — live in
// the sibling x/addrs package.
package configs

import (
	"github.com/hashicorp/hcl/v2"

	"github.com/opentofu/opentofu/internal/configs"
)

// Core configuration types.
type Config = configs.Config
type Module = configs.Module
type Resource = configs.Resource
type Variable = configs.Variable
type Local = configs.Local
type Output = configs.Output
type ModuleCall = configs.ModuleCall
type Provider = configs.Provider
type ProviderConfigRef = configs.ProviderConfigRef
type Backend = configs.Backend
type RequiredProviders = configs.RequiredProviders
type RequiredProvider = configs.RequiredProvider
type ManagedResource = configs.ManagedResource

// Action and ActionTrigger surface the Terraform-actions config parsing
// (top-level `action` blocks + lifecycle `action_trigger`), and
// ActionTriggerDecl the top-level address-targeted trigger form.
type Action = configs.Action
type ActionTrigger = configs.ActionTrigger
type ActionTriggerDecl = configs.ActionTriggerDecl
type ActionTriggerEvent = configs.ActionTriggerEvent
type ActionTriggerOnFailure = configs.ActionTriggerOnFailure

// The lifecycle events an action_trigger can name, as written in `events`.
const (
	ActionBeforeCreate  = configs.ActionBeforeCreate
	ActionAfterCreate   = configs.ActionAfterCreate
	ActionBeforeUpdate  = configs.ActionBeforeUpdate
	ActionAfterUpdate   = configs.ActionAfterUpdate
	ActionBeforeDestroy = configs.ActionBeforeDestroy
	ActionAfterDestroy  = configs.ActionAfterDestroy
)

// What an action_trigger's on_failure does when an invocation fails: halt
// (the default), continue, or taint (halt, plus the resource is marked
// tainted for replacement on the next apply).
const (
	ActionOnFailureHalt     = configs.ActionOnFailureHalt
	ActionOnFailureContinue = configs.ActionOnFailureContinue
	ActionOnFailureTaint    = configs.ActionOnFailureTaint
)

// The state-motion blocks. Import is an instruction to adopt an existing
// remote object into state at plan time rather than create it; Moved renames
// an object's address; Removed forgets one (or destroys it) without a config
// declaration left behind.
type Import = configs.Import
type Moved = configs.Moved
type Removed = configs.Removed

// Check is a `check {}` block: assertions evaluated at the end of a plan or
// apply, reported as warnings rather than errors.
type Check = configs.Check

type StaticModuleCall = configs.StaticModuleCall

// VariableParsingMode selects how a string-form variable value (a -var
// equivalent, a TF_VAR_ environment variable) is parsed: primitive-typed
// variables take the raw string literally, everything else parses it as an
// HCL expression. Each declared Variable carries its mode in .ParsingMode;
// call mode.Parse(name, rawString) to get the value.
type VariableParsingMode = configs.VariableParsingMode

const (
	VariableParseLiteral = configs.VariableParseLiteral
	VariableParseHCL     = configs.VariableParseHCL
)

// CheckRule is one `validation {}` block of a variable declaration (also used
// by check blocks and pre/postconditions): a condition expression plus an
// error_message expression. A Variable carries its rules in .Validations.
type CheckRule = configs.CheckRule

// HCL expression types (from hashicorp/hcl/v2).
type Expression = hcl.Expression
type Body = hcl.Body
type Traversal = hcl.Traversal
