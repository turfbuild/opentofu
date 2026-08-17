// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

// Package dag is a stable boundary over OpenTofu's internal dag package — the
// graph engine OpenTofu's own planner builds on. It re-exports the acyclic
// graph, the concurrent Walker (node-completion-driven, the shape upstream's
// graph walk uses), and the supporting vertex/edge/set types, so a consumer
// can drive an ordered pass with upstream's walk semantics instead of
// mirroring them.
//
// The one seam smoothed over is diagnostics: the Walker's callback speaks
// tfdiags.Diagnostics, which consumers of this facade do not import. The
// Diagnostics alias plus DiagnosticsFromError/(DiagnosticsError) cover the
// producer and consumer side of that type without exposing the package.
package dag

import (
	"github.com/opentofu/opentofu/internal/dag"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Core graph types.
type (
	// Graph is a mutable directed graph: Add vertices, Connect edges.
	Graph = dag.Graph
	// AcyclicGraph embeds Graph and adds the DAG operations — Ancestors,
	// Descendents, TopologicalOrder, Cycles, Validate, TransitiveReduction,
	// and the concurrent Walk.
	AcyclicGraph = dag.AcyclicGraph
	// Vertex is any value usable as a graph node (comparable identity).
	Vertex = dag.Vertex
	// Edge is a directed edge; obtain one from BasicEdge.
	Edge = dag.Edge
	// Set is the vertex set type returned by Ancestors/Descendents.
	Set = dag.Set

	// Walker walks a graph concurrently as vertices become ready: a vertex's
	// callback runs once every dependency's callback has completed. Update
	// feeds it the (possibly growing) graph; Wait blocks until done. This is
	// the engine under OpenTofu's own graph walk.
	Walker = dag.Walker
	// WalkFunc is the per-vertex callback.
	WalkFunc = dag.WalkFunc

	// Diagnostics is the error aggregate a WalkFunc returns. Use
	// DiagnosticsFromError to produce one from a plain error and
	// DiagnosticsError to read one back.
	Diagnostics = tfdiags.Diagnostics
)

// BasicEdge returns a directed edge from source to target.
func BasicEdge(source, target Vertex) Edge {
	return dag.BasicEdge(source, target)
}

// DiagnosticsFromError lifts a plain error into the Diagnostics a WalkFunc
// returns. A nil error yields nil Diagnostics (success).
func DiagnosticsFromError(err error) Diagnostics {
	var diags Diagnostics
	if err != nil {
		diags = diags.Append(err)
	}
	return diags
}

// DiagnosticsError flattens Diagnostics back to a single error, or nil when
// the diagnostics carry no errors.
func DiagnosticsError(diags Diagnostics) error {
	if !diags.HasErrors() {
		return nil
	}
	return diags.Err()
}
