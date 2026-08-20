// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

// Package refactoring is a stable boundary over OpenTofu's internal
// refactoring package, which implements the state-motion semantics of the
// `moved {}` block: finding the statements in a configuration, inferring the
// implied count↔no-count statements, and executing them against a state.
//
// Move endpoints cannot be interpreted outside this package — the internals of
// addrs.MoveEndpoint and addrs.MoveEndpointInModule are unexported, and the
// endpoint unification, chaining, and instance-key expansion rules live with
// them. A consumer therefore drives the whole machine from here rather than
// reimplementing any part of it.
package refactoring

import (
	"github.com/opentofu/opentofu/internal/refactoring"
)

// MoveStatement is a single from→to move, either declared by a `moved {}`
// block (Implied false) or inferred from a count/no-count change (Implied
// true).
type MoveStatement = refactoring.MoveStatement

// MoveResults describes the outcome of ApplyMoves: Changes maps each final
// resource-instance address to the MoveSuccess that put it there (a chain
// A→B→C collapses to a single A→C entry), and Blocked records moves that
// could not happen because an object already occupied the destination.
type (
	MoveResults = refactoring.MoveResults
	MoveSuccess = refactoring.MoveSuccess
	MoveBlocked = refactoring.MoveBlocked
)

// FindMoveStatements returns every `moved {}` block declared anywhere in the
// given configuration tree, in a deterministic but undefined order.
var FindMoveStatements = refactoring.FindMoveStatements

// ImpliedMoveStatements returns the additional statements implied by adding or
// removing `count` on a resource or module call that already has state
// (`a` ↔ `a[0]`), skipping anything an explicit statement already covers.
var ImpliedMoveStatements = refactoring.ImpliedMoveStatements

// ApplyMoves relocates objects within the given state in place, in dependency
// order, and reports what moved and what was blocked. It expects exclusive
// access to the state while it runs.
//
// ApplyMoves has no error return: an unresolvable statement is ignored, and an
// invalid statement graph makes the whole call a no-op. Call
// ValidateMoveStatementGraph first so that second case surfaces as an error
// rather than as silently unmoved state.
var ApplyMoves = refactoring.ApplyMoves

// ValidateMoveStatementGraph reports cycles and self-references among the
// given statements — the conditions under which ApplyMoves declines to move
// anything at all. Other move validity rules depend on the instance set that
// only a plan walk produces, and so are not checked here.
func ValidateMoveStatementGraph(stmts []MoveStatement) error {
	diags := refactoring.ValidateMoveStatementGraph(stmts)
	if diags.HasErrors() {
		return diags.Err()
	}
	return nil
}
