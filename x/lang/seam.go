// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package lang

import (
	"context"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	otflang "github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/tfdiags"

	xaddrs "github.com/opentofu/opentofu/x/addrs"
)

// This file opens the evaluation seam.
//
// The Scope type in scope.go is a closed, concrete, map-shaped scope: a
// consumer hands it flat maps keyed "type.name" and gets values back. That is
// convenient for a caller holding decoded JSON, but it forecloses three things
// a full consumer needs — module-instance addressing, the scope settings
// OpenTofu itself sets per evaluation (BaseDir, PureOnly, SelfAddr), and above
// all an *hcl.EvalContext, without which the canonical typed decode
//
//	hcldec.Decode(body, schema.DecoderSpec(), evalCtx)
//
// cannot be performed at all. A consumer denied that has to reimplement decode
// over untyped maps, inferring cty types back out of Go values.
//
// So this file exposes the interface OpenTofu's evaluator actually consumes,
// and lets a consumer implement it. The map-shaped Scope remains, unchanged
// and still the easy path; EvalScope is the complete one.

// Data is the interface an evaluation backend implements to resolve
// references. It is OpenTofu's own lang.Data: implement it and OpenTofu's
// evaluator will drive it exactly as it drives its own.
//
// Every method is passed the source range of the reference being resolved, so
// an implementation can attribute a diagnostic to the expression that caused
// it. Return cty.DynamicVal (not an error) for a reference that is legitimately
// not yet known — unknowns are values in cty, and propagating one is how
// "known after apply" works.
type Data = otflang.Data

// ParseRef controls which reference types a scope admits, by parsing an HCL
// traversal into a typed reference. DefaultParseRef accepts all of them.
type ParseRef = otflang.ParseRef

// Diagnostics and SourceRange are named in Data's method set, so an
// implementation outside this module needs both.
type (
	Diagnostics = tfdiags.Diagnostics
	SourceRange = tfdiags.SourceRange
)

// DefaultParseRef accepts every standard reference type, which is the correct
// choice for evaluating a root or child module. A caller wanting to *reject*
// some reference class in a particular position supplies its own instead.
var DefaultParseRef ParseRef = addrs.ParseRef

// EvalScope is a caller-configured evaluation scope. Unlike Scope it holds no
// values of its own: resolution is delegated entirely to Data, so the caller
// decides where values come from — a whole-state snapshot, a per-instance
// lookup, a durable key-value store, whatever the consumer's storage model is.
//
// Zero value is not usable: Data is required. ParseRef defaults to
// DefaultParseRef when nil.
type EvalScope struct {
	// Data resolves references. Required.
	Data Data

	// ParseRef controls which references the scope admits. Defaults to
	// DefaultParseRef.
	ParseRef ParseRef

	// SelfAddr is what `self` aliases, or nil to make `self` unavailable.
	// Set this when evaluating a position where self is meaningful — a
	// provisioner, a destroy-time reference — and leave it nil otherwise, so
	// that a stray `self` is an error rather than silently resolving.
	SelfAddr xaddrs.Referenceable

	// SourceAddr is the address of the item being evaluated, which governs
	// access to anything scoped to that item. Nil means module-level access.
	SourceAddr xaddrs.Referenceable

	// Caller is what `caller` resolves to: the triggering resource instance's
	// value when evaluating an action's configuration for an action_trigger.
	// cty.NilVal makes `caller` unavailable, so a reference to it anywhere
	// else is an error rather than a silent null.
	//
	// Unlike everything else a reference can name, this one is not resolved
	// through Data. The triggering instance is not addressable from inside
	// the action's configuration — `caller` is an alias, not an address — so
	// there is nothing for a Data implementation to look up, and the value
	// is carried directly. Scope carries it the same way.
	Caller cty.Value

	// BaseDir is the directory that filesystem functions — file(),
	// templatefile(), fileexists() — resolve relative paths against. Leaving
	// it empty resolves them against the process working directory, which for
	// a long-lived server is essentially never what the configuration author
	// meant. Set it to the module directory.
	BaseDir string

	// PureOnly makes impure functions (timestamp(), uuid()) return unknown
	// rather than executing. Set it during plan so a value cannot be baked in
	// at plan time and then differ at apply.
	PureOnly bool

	// PlanTimestamp is what the plantimestamp() function returns.
	PlanTimestamp time.Time

	// ConsoleMode includes console-only functions.
	ConsoleMode bool

	// ProviderFunctions resolves provider-defined functions. Nil leaves them
	// unavailable, and a configuration calling one gets a diagnostic.
	ProviderFunctions ProviderFunction
}

// ProviderFunction resolves a provider-defined function to its implementation.
type ProviderFunction = otflang.ProviderFunction

// otf builds the OpenTofu scope this EvalScope describes.
func (e *EvalScope) otf() *otflang.Scope {
	parseRef := e.ParseRef
	if parseRef == nil {
		parseRef = DefaultParseRef
	}
	return &otflang.Scope{
		Data:              e.Data,
		ParseRef:          parseRef,
		SelfAddr:          e.SelfAddr,
		SourceAddr:        e.SourceAddr,
		CallerValue:       e.Caller,
		BaseDir:           e.BaseDir,
		PureOnly:          e.PureOnly,
		PlanTimestamp:     e.PlanTimestamp,
		ConsoleMode:       e.ConsoleMode,
		ProviderFunctions: e.ProviderFunctions,
	}
}

// EvalExpr evaluates a single expression, converting the result to wantType.
// Pass cty.DynamicPseudoType to accept whatever type the expression produces.
func (e *EvalScope) EvalExpr(ctx context.Context, expr hcl.Expression, wantType cty.Type) (cty.Value, Diagnostics) {
	return e.otf().EvalExpr(ctx, expr, wantType)
}

// EvalContext builds the *hcl.EvalContext for a set of references — the thing
// that makes a native typed decode possible:
//
//	traversals := hcldec.Variables(body, spec)
//	refs, diags := lang.References(parseRef, traversals)
//	evalCtx, moreDiags := scope.EvalContext(ctx, refs)
//	val, decDiags := hcldec.Decode(body, spec, evalCtx)
//
// which yields a cty.Value carrying the schema's own types, rather than values
// whose types have to be inferred after the fact.
func (e *EvalScope) EvalContext(ctx context.Context, refs []*xaddrs.Reference) (*hcl.EvalContext, Diagnostics) {
	return e.otf().EvalContext(ctx, refs)
}

// EvalContextWithParent is EvalContext with an outer scope, for evaluating an
// expression nested inside one that already bound variables — `dynamic` blocks
// and `for` expressions being the cases that need it.
func (e *EvalScope) EvalContextWithParent(ctx context.Context, parent *hcl.EvalContext, refs []*xaddrs.Reference) (*hcl.EvalContext, Diagnostics) {
	return e.otf().EvalContextWithParent(ctx, parent, refs)
}

// References parses HCL traversals into typed references. Obtain the
// traversals with hcldec.Variables(body, spec) for a schema-driven decode, or
// expr.Variables() for a single expression.
func References(parseRef ParseRef, traversals []hcl.Traversal) ([]*xaddrs.Reference, Diagnostics) {
	if parseRef == nil {
		parseRef = DefaultParseRef
	}
	return otflang.References(parseRef, traversals)
}

// ReferencesInExpr returns the references a single expression makes.
func ReferencesInExpr(parseRef ParseRef, expr hcl.Expression) ([]*xaddrs.Reference, Diagnostics) {
	if parseRef == nil {
		parseRef = DefaultParseRef
	}
	return otflang.ReferencesInExpr(parseRef, expr)
}
