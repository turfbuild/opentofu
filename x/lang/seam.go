// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package lang

import (
	"context"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	otflang "github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/tfdiags"

	xaddrs "github.com/opentofu/opentofu/x/addrs"
)

// This file is the evaluation surface of this package.
//
// EvalScope is what evaluates. It exposes the interface OpenTofu's own
// evaluator consumes — resolution delegated to a caller-supplied Data — so a
// consumer can back it with whatever storage model it has, address module
// instances, set the per-evaluation settings OpenTofu sets (BaseDir, PureOnly,
// SelfAddr), and above all obtain the *hcl.EvalContext that the canonical
// typed decode
//
//	hcldec.Decode(body, schema.DecoderSpec(), evalCtx)
//
// requires — or, better, EvalBlock, which runs that decode the way OpenTofu
// itself does. Without that a consumer has to reimplement decode over untyped
// maps, inferring cty types back out of Go values.
//
// There is one evaluator and one place where OpenTofu's scope is constructed:
// the otf method below. Every entry point in this package goes through it, so
// a capability added to the scope reaches all of them at once.

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
// traversal into a typed reference. A nil ParseRef, wherever one is taken,
// accepts every standard reference type, which is the correct choice for
// evaluating a root or child module; a caller wanting to *reject* some
// reference class in a particular position supplies its own instead.
type ParseRef = otflang.ParseRef

// Diagnostics and SourceRange are named in Data's method set, so an
// implementation outside this module needs both.
type (
	Diagnostics = tfdiags.Diagnostics
	SourceRange = tfdiags.SourceRange
)

// defaultParseRef is what a nil ParseRef means.
var defaultParseRef ParseRef = addrs.ParseRef

// EvalScope is a caller-configured evaluation scope. It holds no values of
// its own: resolution is delegated entirely to Data, so the caller
// decides where values come from — a whole-state snapshot, a per-instance
// lookup, a durable key-value store, whatever the consumer's storage model is.
//
// Zero value is not usable: Data is required. A nil ParseRef accepts every
// standard reference type.
type EvalScope struct {
	// Data resolves references. Required.
	Data Data

	// ParseRef controls which references the scope admits. Nil admits every
	// standard reference type.
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
	// is carried directly.
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
		parseRef = defaultParseRef
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

// EvalBlock evaluates a configuration body against a block schema and returns
// an object conforming to the schema's implied type — the whole of
// OpenTofu's own block decode: the references the schema expects are
// resolved, the body is decoded against the schema's spec, attributes written
// in block syntax are fixed up, a reference to an ephemeral value outside an
// ephemeral context is refused, and deprecated attributes are reported.
//
// It does not expand `dynamic` blocks; pass the body through ExpandBlock
// first when the configuration may contain them:
//
//	body, diags := scope.ExpandBlock(ctx, body, schema)
//	val, moreDiags := scope.EvalBlock(ctx, body, schema)
func (e *EvalScope) EvalBlock(ctx context.Context, body hcl.Body, schema *configschema.Block) (cty.Value, Diagnostics) {
	return e.otf().EvalBlock(ctx, body, schema)
}

// ExpandBlock expands every `dynamic` block in body, returning a body ready
// for EvalBlock. The for_each of each dynamic block is evaluated in this
// scope.
func (e *EvalScope) ExpandBlock(ctx context.Context, body hcl.Body, schema *configschema.Block) (hcl.Body, Diagnostics) {
	return e.otf().ExpandBlock(ctx, body, schema)
}

// References parses HCL traversals into typed references. Obtain the
// traversals with hcldec.Variables(body, spec) for a schema-driven decode, or
// expr.Variables() for a single expression.
func References(parseRef ParseRef, traversals []hcl.Traversal) ([]*xaddrs.Reference, Diagnostics) {
	if parseRef == nil {
		parseRef = defaultParseRef
	}
	return otflang.References(parseRef, traversals)
}

// ReferencesInBlock returns the references a configuration body makes, found
// where the block schema expects expressions — including inside `dynamic`
// blocks, whose iterator references it resolves the way ExpandBlock will.
func ReferencesInBlock(parseRef ParseRef, body hcl.Body, schema *configschema.Block) ([]*xaddrs.Reference, Diagnostics) {
	if parseRef == nil {
		parseRef = defaultParseRef
	}
	return otflang.ReferencesInBlock(parseRef, body, schema)
}

// ReferencesInExpr returns the references a single expression makes.
func ReferencesInExpr(parseRef ParseRef, expr hcl.Expression) ([]*xaddrs.Reference, Diagnostics) {
	if parseRef == nil {
		parseRef = defaultParseRef
	}
	return otflang.ReferencesInExpr(parseRef, expr)
}
