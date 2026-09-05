// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package lang

import (
	"context"
	"errors"
	"sort"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/evalchecks"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// This file is the facade over OpenTofu's repetition meta-argument checks
// (internal/lang/evalchecks) — the rules that decide how many instances a
// `count` / `for_each` / `enabled` argument declares, and which values are
// unusable for that purpose. It mirrors the adapters OpenTofu itself uses in
// internal/tofu/eval_expansion.go, differing in one deliberate way described
// under "Deferral" below.
//
// # Deferral
//
// OpenTofu treats a meta-argument that is not yet known as plan-fatal: it has
// no mechanism to come back later, so it raises an error suggesting -target or
// -exclude. A library consumer that converges over several plan/apply rounds
// wants the opposite — "not knowable yet" is a retriable condition, distinct
// from "this value can never work". Every function here therefore returns a
// three-way outcome:
//
//	(result, false, nil)  the argument is settled; use the result
//	(zero,   true,  nil)  not knowable yet; retry after applying upstreams
//	(zero,   false, err)  a permanent configuration error
//
// The knownness bar that separates the second case from the first is not a
// simple IsKnown, and getting it wrong in either direction is a bug:
//
//   - maps and objects: only the top level must be known. The keys become the
//     instance keys; unknown *values* pass through as each.value, and whatever
//     reads one defers on its own. A deep check here would wrongly defer a
//     perfectly enumerable call.
//   - sets: the elements *are* the keys, so an unknown element means the
//     instance addresses are unknowable. Requires wholly-known.
//
// Rather than restate that, EvaluateForEach passes allowUnknown=true to
// evalchecks and reads the answer back off the returned value: evalchecks
// applies exactly this bar internally (performTypeAndValueChecks), and with
// unknowns allowed it reports "not determinable" by returning an unknown value
// with no diagnostics, while still diagnosing every permanent problem.
//
// # What signals a deferral
//
// The signal is an unknown *value*, never a failed evaluation. evalchecks
// reports "not determinable" by returning an unknown value with no diagnostics
// — always for count, and for for_each whenever unknowns are allowed — so a
// diagnostic coming back from it can only mean a real problem, and is raised.
//
// The distinction is not academic, because the scope decides what an
// unresolvable reference looks like. A permissive Data resolves one to
// cty.DynamicVal, which is a value and correctly defers. A strict Data
// diagnoses it, and those diagnostics say things like "the walk reached a body
// that reads it before it was planned or fetched, which is a defect in the
// walk's ordering". Treating a failed evaluation as a deferral would swallow
// exactly those, converting a caller's ordering bug into a resource that
// silently defers every round and never converges.
//
// EvaluateEnabled is the one exception, and it is forced: OpenTofu has no
// allow-unknown mode for that argument and diagnoses an unknown as an error,
// tagged indistinguishably from the sensitive case. Knownness is therefore
// tested before the check runs — but a failed evaluation there is still an
// error, not a deferral.

// ForEachPair is one repetition produced by a for_each argument.
//
// Key is a cty.Value rather than a string because the tuple/list forms that
// import blocks accept are keyed by index, not by name; for the map, object,
// and set forms that resources and module calls accept it is always a string.
type ForEachPair struct {
	Key   cty.Value
	Value cty.Value
}

// contextFunc adapts a scope to the hcl.EvalContext constructor evalchecks
// wants for expressions it evaluates itself.
func contextFunc(scope *EvalScope) evalchecks.ContextFunc {
	return func(refs []*addrs.Reference) (*hcl.EvalContext, tfdiags.Diagnostics) {
		return scope.EvalContext(context.Background(), refs)
	}
}

// EvaluateForEach evaluates a `for_each` argument against scope and returns its
// repetitions in a stable order, applying OpenTofu's own type and value rules.
//
// allowTuple admits the tuple and list forms, which OpenTofu accepts only for
// import blocks. allowUnknown selects the deferral behavior: with it set, a
// for_each whose instance keys are not yet determinable reports deferred=true;
// without it, that same case is a permanent error — the right answer where the
// keys must be settled now, as when deciding which object an import adopts.
//
// Keys of the map, object, and set forms are sorted; the tuple and list forms
// keep their index order.
func EvaluateForEach(expr hcl.Expression, scope *EvalScope, allowTuple, allowUnknown bool) ([]ForEachPair, bool, error) {
	if expr == nil {
		return nil, false, nil
	}
	val, diags := evalchecks.EvaluateForEachExpressionValue(expr, contextFunc(scope), allowUnknown, allowTuple, nil)
	if diags.HasErrors() {
		return nil, false, diagsError(diags)
	}
	if !val.IsKnown() {
		// Only reachable with allowUnknown; otherwise evalchecks diagnosed it.
		return nil, true, nil
	}
	if val.IsNull() || val.LengthInt() == 0 {
		return nil, false, nil
	}

	// Marks are gone as far as the checks are concerned — evalchecks rejects
	// sensitive and ephemeral outright and strips deprecation marks — but
	// ElementIterator and AsString panic on any mark at all, so unmark before
	// enumerating rather than trusting that no other mark can reach here.
	val, _ = val.UnmarkDeep()

	pairs := make([]ForEachPair, 0, val.LengthInt())
	for it := val.ElementIterator(); it.Next(); {
		k, v := it.Element()
		pairs = append(pairs, ForEachPair{Key: k, Value: v})
	}
	if ty := val.Type(); ty.IsTupleType() || ty.IsListType() {
		// Index-keyed: the iterator already yields them in order.
		return pairs, false, nil
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].Key.AsString() < pairs[j].Key.AsString()
	})
	return pairs, false, nil
}

// EvaluateCount evaluates a `count` argument against scope and returns the
// number of instances it declares.
//
// A count that is not yet known reports deferred=true. Sensitive values are
// permitted (a count cannot disclose the value it was derived from, only its
// magnitude, and an instance key is a number either way); ephemeral values are
// not. Both of those are OpenTofu's rules, not this wrapper's.
func EvaluateCount(expr hcl.Expression, scope *EvalScope) (int, bool, error) {
	if expr == nil {
		return 0, false, nil
	}
	// cty.Number as the want type is what makes HCL's own conversion apply, so
	// a count that arrives as the string "2" — which is how a JSON-shaped
	// caller serializes it — reads as 2. This matches the want type OpenTofu
	// passes (internal/tofu/eval_expansion.go); the conversion evalchecks does
	// internally, via gocty, would not accept it.
	evaluate := func(expr hcl.Expression) (cty.Value, tfdiags.Diagnostics) {
		return scope.EvalExpr(context.Background(), expr, cty.Number)
	}

	val, diags := evalchecks.EvaluateCountExpressionValue(expr, evaluate)
	if diags.HasErrors() {
		return 0, false, diagsError(diags)
	}
	if !val.IsKnown() {
		return 0, true, nil
	}
	n, _ := val.AsBigFloat().Int64()
	return int(n), false, nil
}

// EvaluateEnabled evaluates an `enabled` argument against scope, reporting
// whether the object it governs is declared at all.
//
// An enabled value that is not yet known reports deferred=true. OpenTofu has no
// allow-unknown mode for this argument — and its unknown diagnostic is not
// distinguishable from its sensitive one, both being tagged as caused by
// unknown values — so the knownness test is a pre-pass here. It is a plain
// IsKnown: a bool has no interior to be partially known. An expression that
// fails to evaluate at all is an error rather than a deferral; only an unknown
// value defers.
func EvaluateEnabled(expr hcl.Expression, scope *EvalScope) (bool, bool, error) {
	if expr == nil {
		return true, false, nil
	}
	val, diags := scope.EvalExpr(context.Background(), expr, cty.DynamicPseudoType)
	if diags.HasErrors() {
		return false, false, diagsError(diags)
	}
	if !val.IsKnown() {
		return false, true, nil
	}

	enabled, diags := evalchecks.EvaluateEnabledExpression(expr, contextFunc(scope))
	if diags.HasErrors() {
		return false, false, diagsError(diags)
	}
	return enabled, false, nil
}

// diagsError renders check diagnostics as an error. tfdiags.Diagnostics.Err
// returns nil when there is nothing to report, which the callers have already
// ruled out; the fallback keeps a nil error from escaping as a success.
func diagsError(diags tfdiags.Diagnostics) error {
	if err := diags.Err(); err != nil {
		return err
	}
	return errors.New("invalid repetition argument")
}
