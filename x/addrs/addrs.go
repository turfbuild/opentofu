// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

// Package addrs is a stable boundary over OpenTofu's internal addrs package.
// It re-exports the canonical resource-addressing types and wraps the OpenTofu
// parsers/formatters so consumers can parse, format, and reason about
// addresses without importing OpenTofu internals (or tfdiags) directly.
package addrs

import (
	"fmt"

	"github.com/opentofu/opentofu/internal/addrs"
)

// Address types.
type (
	AbsResource         = addrs.AbsResource
	AbsResourceInstance = addrs.AbsResourceInstance
	Resource            = addrs.Resource
	ResourceInstance    = addrs.ResourceInstance
	ConfigResource      = addrs.ConfigResource
	ModuleInstance      = addrs.ModuleInstance
	ModuleInstanceStep  = addrs.ModuleInstanceStep
	InstanceKey         = addrs.InstanceKey
	IntKey              = addrs.IntKey
	StringKey           = addrs.StringKey
	ResourceMode        = addrs.ResourceMode
	AbsOutputValue      = addrs.AbsOutputValue
	OutputValue         = addrs.OutputValue
	Module              = addrs.Module
)

// Move endpoints — the two halves of a `moved {}` block as written in
// configuration. MoveEndpoint's internals are unexported, so the only thing a
// consumer can do with one is render it (String) or resolve it against the
// module it was declared in (ConfigMoveable), which yields either a
// ConfigResource or a Module.
type (
	MoveEndpoint   = addrs.MoveEndpoint
	ConfigMoveable = addrs.ConfigMoveable
)

// Reference subjects — the typed forms an expression reference can name.
// Referenceable is the interface every subject implements; the rest are the
// concrete subjects a consumer type-switches over. Exposed so a consumer can
// classify a reference from its typed subject instead of re-parsing the
// rendered string.
type (
	Referenceable            = addrs.Referenceable
	InputVariable            = addrs.InputVariable
	LocalValue               = addrs.LocalValue
	ModuleCall               = addrs.ModuleCall
	ModuleCallInstance       = addrs.ModuleCallInstance
	ModuleCallOutput         = addrs.ModuleCallOutput
	ModuleCallInstanceOutput = addrs.ModuleCallInstanceOutput
)

// Resource modes.
const (
	ManagedResourceMode = addrs.ManagedResourceMode
	DataResourceMode    = addrs.DataResourceMode
)

// NoKey is the instance key for resources without count or for_each.
var NoKey = addrs.NoKey

// RootModuleInstance is the address of the root module.
var RootModuleInstance = addrs.RootModuleInstance

// ParseAbsResourceInstance parses an absolute resource-instance address string
// such as "aws_instance.x", "data.aws_ami.latest", "module.foo[0].aws_instance.x",
// or `module.foo["a"].module.bar.aws_instance.x[2]`.
// The first error from the underlying parser is returned as a regular Go error
// so callers don't need to import OpenTofu's tfdiags package.
func ParseAbsResourceInstance(str string) (AbsResourceInstance, error) {
	addr, diags := addrs.ParseAbsResourceInstanceStr(str)
	if diags.HasErrors() {
		return AbsResourceInstance{}, diags.Err()
	}
	return addr, nil
}

// ParseAbsResource parses an absolute resource address (no instance key), such
// as "aws_instance.x" or "module.foo.aws_instance.x".
func ParseAbsResource(str string) (AbsResource, error) {
	addr, diags := addrs.ParseAbsResourceStr(str)
	if diags.HasErrors() {
		return AbsResource{}, diags.Err()
	}
	return addr, nil
}

// ParseModuleInstance parses a module-instance address such as "module.foo" or
// `module.foo[0].module.bar["k"]`. The empty string parses as the root module.
func ParseModuleInstance(str string) (ModuleInstance, error) {
	if str == "" {
		return RootModuleInstance, nil
	}
	addr, diags := addrs.ParseModuleInstanceStr(str)
	if diags.HasErrors() {
		return nil, diags.Err()
	}
	return addr, nil
}

// FormatInstanceKeySuffix returns the bracketed instance-key suffix used when
// rendering a resource or module-instance address. NoKey produces "".
// IntKey(0) produces "[0]"; StringKey("k") produces `["k"]` with HCL-correct
// escaping.
func FormatInstanceKeySuffix(k InstanceKey) string {
	if k == NoKey {
		return ""
	}
	return k.String()
}

// InstanceKeyFromAny converts an HCL-eval value (typically float64, int, or
// string) into a typed InstanceKey. A nil value maps to NoKey.
func InstanceKeyFromAny(v any) (InstanceKey, error) {
	switch k := v.(type) {
	case nil:
		return NoKey, nil
	case InstanceKey:
		return k, nil
	case int:
		return IntKey(k), nil
	case int64:
		return IntKey(int(k)), nil
	case float64:
		return IntKey(int(k)), nil
	case string:
		return StringKey(k), nil
	default:
		return NoKey, fmt.Errorf("instance key has unsupported type %T", v)
	}
}

// IsDataAddr reports whether the given resource-instance address refers to a
// data source. Returns false for unparseable input.
func IsDataAddr(str string) bool {
	addr, err := ParseAbsResourceInstance(str)
	if err != nil {
		return false
	}
	return addr.Resource.Resource.Mode == DataResourceMode
}
