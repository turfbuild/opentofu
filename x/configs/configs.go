// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

// Package configs re-exports OpenTofu's internal configuration types through a stable API boundary.
// This enables configuration parsing and analysis without direct imports of OpenTofu internals.
package configs

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
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

// Action and ActionTrigger surface the OTF fork's native Terraform-actions
// config parsing (top-level `action` blocks + lifecycle `action_trigger`).
type Action = configs.Action
type ActionTrigger = configs.ActionTrigger
type ActionTriggerEvent = configs.ActionTriggerEvent
type Removed = configs.Removed

// Import is the `import {}` state-motion block: an instruction to adopt an
// existing remote object into state at plan time rather than create it.
type Import = configs.Import
type ProviderMeta = configs.ProviderMeta
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

// Parser for HCL configuration files.
type Parser = configs.Parser

// Constructors.
var NewParser = configs.NewParser
var NewModule = configs.NewModule
var BuildConfig = configs.BuildConfig

// HCL expression types (from hashicorp/hcl/v2).
type Expression = hcl.Expression
type Body = hcl.Body
type Traversal = hcl.Traversal
type TraverseRoot = hcl.TraverseRoot
type TraverseAttr = hcl.TraverseAttr
type TraverseIndex = hcl.TraverseIndex

// Address types for configuration references.
type ModuleAddr = addrs.Module
type ModuleSource = addrs.ModuleSource
type ProviderAddr = addrs.Provider
type Reference = addrs.Reference
type Referenceable = addrs.Referenceable

// Resource modes (re-export from addrs).
type ResourceMode = addrs.ResourceMode

const (
	ManagedResourceMode = addrs.ManagedResourceMode
	DataResourceMode    = addrs.DataResourceMode
	ModuleAddrType      = addrs.ModuleAddrType
)

// RootModule is the address of the root configuration module.
var RootModule = addrs.RootModule
