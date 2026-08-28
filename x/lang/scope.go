// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

// Package lang provides expression evaluation capabilities for infrastructure configuration.
// It wraps OpenTofu's internal lang package to evaluate HCL expressions with state context.
package lang

import (
	"github.com/zclconf/go-cty/cty"
)

// Scope provides the context for evaluating HCL expressions.
// It holds references to state data needed for resolving resource references.
type Scope struct {
	// Variables holds input variable values (var.*)
	Variables map[string]cty.Value

	// Locals holds local value definitions (local.*)
	Locals map[string]cty.Value

	// Resources holds resource state values (type.name.*)
	// Key format: "type.name" (e.g., "aws_instance.web"). Entries under other
	// key shapes (e.g. full state addresses with a "module." prefix) may be
	// stored, but only local-form keys are resolvable from expressions.
	Resources map[string]cty.Value

	// DataSources holds data source values (data.type.name.*)
	// Key format: "type.name" (e.g., "aws_ami.latest")
	DataSources map[string]cty.Value

	// Ephemerals holds opened ephemeral resource values (ephemeral.type.name.*)
	// Key format: "type.name" (e.g., "vault_kv_secret.creds"), the same shape
	// DataSources uses — the block type is the store, not part of the key.
	//
	// The values here are Ephemeral-marked, which is what stops one from being
	// written into anything that persists. Nothing in this map is ever
	// serialized: an ephemeral resource has no state slot at all.
	Ephemerals map[string]cty.Value

	// Outputs holds module output values (output.* or module.*.*)
	Outputs map[string]cty.Value

	// Modules holds the evaluated outputs of nested module calls, keyed by
	// the call's local name (e.g., "interfaces" for `module "interfaces"`).
	// Each value is an ObjectVal of the module's output names → values.
	// Populated by the walker after recursing into a child module; consumed
	// by EvalContext to make `module.<name>.<output>` references resolvable.
	Modules map[string]cty.Value

	// Self holds the "self" reference value (for provisioners)
	Self cty.Value

	// Each holds the "each" reference values (each.key, each.value)
	Each *EachData

	// Count holds the "count" reference value (count.index)
	Count *int

	// Path holds path attribute values (path.module, path.root, path.cwd)
	Path PathData

	// Workspace is the value `terraform.workspace` resolves to: the name of
	// the OpenTofu workspace (state slot) this walk is planning against.
	//
	// Empty means "default", which is both OpenTofu's own default workspace
	// name and the safe answer for a caller that has not plumbed one through —
	// the adapter substitutes it, so an unset field cannot produce an empty
	// string where a workspace name is expected.
	Workspace string
}

// EachData holds values for for_each iteration.
type EachData struct {
	Key   cty.Value
	Value cty.Value
}

// PathData holds path attribute values.
type PathData struct {
	Module string
	Root   string
	Cwd    string
}

// NewScope creates a new evaluation scope.
func NewScope() *Scope {
	return &Scope{
		Variables:   make(map[string]cty.Value),
		Locals:      make(map[string]cty.Value),
		Resources:   make(map[string]cty.Value),
		DataSources: make(map[string]cty.Value),
		Ephemerals:  make(map[string]cty.Value),
		Outputs:     make(map[string]cty.Value),
		Modules:     make(map[string]cty.Value),
	}
}

// SetVariable sets an input variable value.
func (s *Scope) SetVariable(name string, val cty.Value) {
	s.Variables[name] = val
}

// SetLocal sets a local value.
func (s *Scope) SetLocal(name string, val cty.Value) {
	s.Locals[name] = val
}

// SetResource sets a resource's state values.
// addr should be in the format "type.name" (e.g., "aws_instance.web").
func (s *Scope) SetResource(addr string, val cty.Value) {
	s.Resources[addr] = val
}

// RemoveResource removes a resource from the scope.
// addr should be in the format "type.name" (e.g., "aws_instance.web").
func (s *Scope) RemoveResource(addr string) {
	delete(s.Resources, addr)
}

// SetDataSource sets a data source's values.
// addr should be in the format "type.name" (e.g., "aws_ami.latest").
func (s *Scope) SetDataSource(addr string, val cty.Value) {
	s.DataSources[addr] = val
}

// RemoveDataSource removes a data source from the scope.
// addr should be in the format "type.name" (e.g., "aws_ami.latest").
func (s *Scope) RemoveDataSource(addr string) {
	delete(s.DataSources, addr)
}

// SetEphemeral sets an opened ephemeral resource's value.
// addr should be in the format "type.name" (e.g., "vault_kv_secret.creds").
func (s *Scope) SetEphemeral(addr string, val cty.Value) {
	s.Ephemerals[addr] = val
}

// RemoveEphemeral removes an ephemeral resource from the scope.
// addr should be in the format "type.name" (e.g., "vault_kv_secret.creds").
func (s *Scope) RemoveEphemeral(addr string) {
	delete(s.Ephemerals, addr)
}

// SetOutput sets an output value.
func (s *Scope) SetOutput(name string, val cty.Value) {
	s.Outputs[name] = val
}

// SetModuleOutput registers the evaluated outputs of a child module call.
// The value's cty type determines how it surfaces in EvalContext:
//
//   - ObjectVal({output_name → value}) — a non-keyed module call. Each output
//     is exposed individually as `module.<modName>.<output_name>`.
//
//   - TupleVal([instance_outputs, …]) or ListVal — a count-keyed module
//     call. The whole tuple/list is exposed at `module.<modName>`, navigable
//     by index: `module.<modName>[0].<output>`, `module.<modName>[*].<output>`.
//
//   - MapVal({key → instance_outputs}) — a for_each-keyed module call.
//     The whole map is exposed at `module.<modName>`, navigable by key:
//     `module.<modName>["foo"].<output>`.
//
// Other cty types are skipped silently. This is the ONLY way a module call
// becomes addressable: matching Terraform/OpenTofu semantics, `module.<name>`
// exposes declared outputs and nothing else — resources inside the module are
// not reachable through the module namespace.
func (s *Scope) SetModuleOutput(modName string, outputs cty.Value) {
	s.Modules[modName] = outputs
}
