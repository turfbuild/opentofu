// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package dag_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/opentofu/opentofu/x/dag"
)

// TestWalkerRunsDependenciesFirst drives a small diamond through the
// concurrent Walker via the facade alone: a → {b, c} → d, asserting every
// callback sees its dependencies completed and that independent vertices may
// overlap.
func TestWalkerRunsDependenciesFirst(t *testing.T) {
	var g dag.AcyclicGraph
	for _, v := range []string{"a", "b", "c", "d"} {
		g.Add(v)
	}
	// Edges run dependent → dependency, and the Walker takes Reverse: true —
	// the exact shape OpenTofu's own graph walk uses (internal/tofu/graph.go).
	g.Connect(dag.BasicEdge("b", "a"))
	g.Connect(dag.BasicEdge("c", "a"))
	g.Connect(dag.BasicEdge("d", "b"))
	g.Connect(dag.BasicEdge("d", "c"))

	var mu sync.Mutex
	done := map[string]bool{}
	w := &dag.Walker{Reverse: true, Callback: func(v dag.Vertex) dag.Diagnostics {
		name := v.(string)
		mu.Lock()
		defer mu.Unlock()
		switch name {
		case "b", "c":
			if !done["a"] {
				return dag.DiagnosticsFromError(fmt.Errorf("%s ran before a", name))
			}
		case "d":
			if !done["b"] || !done["c"] {
				return dag.DiagnosticsFromError(fmt.Errorf("d ran before b and c"))
			}
		}
		done[name] = true
		return nil
	}}
	w.Update(&g)
	if err := dag.DiagnosticsError(w.Wait()); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"a", "b", "c", "d"} {
		if !done[v] {
			t.Errorf("%s never ran", v)
		}
	}
}

// TestCyclesAreReported covers the Validate/Cycles surface the execution
// wiring needs for cycle safety.
func TestCyclesAreReported(t *testing.T) {
	var g dag.AcyclicGraph
	g.Add("a")
	g.Add("b")
	g.Connect(dag.BasicEdge("a", "b"))
	g.Connect(dag.BasicEdge("b", "a"))
	if err := g.Validate(); err == nil {
		t.Fatal("expected a cycle error")
	}
	if cycles := g.Cycles(); len(cycles) != 1 || len(cycles[0]) != 2 {
		t.Fatalf("Cycles() = %v, want one 2-ring", cycles)
	}
}
