// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package refactoring_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	xaddrs "github.com/opentofu/opentofu/x/addrs"
	xconfigs "github.com/opentofu/opentofu/x/configs"
	xrefactoring "github.com/opentofu/opentofu/x/refactoring"
	xstate "github.com/opentofu/opentofu/x/state"
)

// loadConfig writes the given root-module source to a temp directory and loads
// it into the config tree that the refactoring entry points consume.
func loadConfig(t *testing.T, src string) *xconfigs.Config {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(src), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	loader, err := xconfigs.NewLoader(filepath.Join(dir, ".terraform", "modules"))
	if err != nil {
		t.Fatalf("new loader: %v", err)
	}
	cfg, err := loader.LoadConfig(context.Background(), dir, xconfigs.RootModuleCall(dir, "default", nil))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg
}

// stateWith builds a state holding one ready object at each of the given
// resource-instance addresses.
func stateWith(t *testing.T, addrs ...string) *xstate.State {
	t.Helper()
	st := xstate.NewState()
	provider := xstate.RootProviderConfig(xstate.NewProvider("registry.opentofu.org", "hashicorp", "random"))
	for _, s := range addrs {
		addr, err := xaddrs.ParseAbsResourceInstance(s)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		mod := st.EnsureModule(addr.Module)
		mod.SetResourceInstanceCurrent(addr.Resource, &xstate.ResourceInstanceObjectSrc{
			AttrsJSON: []byte(`{"id":"` + s + `"}`),
			Status:    xstate.ObjectReady,
		}, provider, xaddrs.NoKey)
	}
	return st
}

// stateAddrs renders every resource-instance address held in the state, sorted.
func stateAddrs(st *xstate.State) []string {
	var out []string
	for _, mod := range st.Modules {
		for _, res := range mod.Resources {
			for key := range res.Instances {
				out = append(out, res.Addr.Instance(key).String())
			}
		}
	}
	sort.Strings(out)
	return out
}

func TestApplyMoves_Rename(t *testing.T) {
	cfg := loadConfig(t, `
resource "random_pet" "renamed" {}

moved {
  from = random_pet.original
  to   = random_pet.renamed
}
`)
	stmts := xrefactoring.FindMoveStatements(cfg)
	if len(stmts) != 1 {
		t.Fatalf("got %d statements, want 1", len(stmts))
	}
	if err := xrefactoring.ValidateMoveStatementGraph(stmts); err != nil {
		t.Fatalf("validate: %v", err)
	}

	st := stateWith(t, "random_pet.original")
	results := xrefactoring.ApplyMoves(stmts, st)

	if got, want := stateAddrs(st), []string{"random_pet.renamed"}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("state addresses = %v, want %v", got, want)
	}
	changes := results.Changes.Elements()
	if len(changes) != 1 {
		t.Fatalf("got %d changes, want 1", len(changes))
	}
	if got, want := changes[0].Value.From.String(), "random_pet.original"; got != want {
		t.Errorf("moved from %q, want %q", got, want)
	}
	if got, want := changes[0].Value.To.String(), "random_pet.renamed"; got != want {
		t.Errorf("moved to %q, want %q", got, want)
	}
	if results.Blocked.Len() != 0 {
		t.Errorf("got %d blocked moves, want 0", results.Blocked.Len())
	}
}

func TestApplyMoves_Chained(t *testing.T) {
	cfg := loadConfig(t, `
resource "random_pet" "c" {}

moved {
  from = random_pet.a
  to   = random_pet.b
}

moved {
  from = random_pet.b
  to   = random_pet.c
}
`)
	stmts := xrefactoring.FindMoveStatements(cfg)
	st := stateWith(t, "random_pet.a")
	results := xrefactoring.ApplyMoves(stmts, st)

	if got := stateAddrs(st); len(got) != 1 || got[0] != "random_pet.c" {
		t.Errorf("state addresses = %v, want [random_pet.c]", got)
	}
	// A chain collapses to a single entry naming the original source.
	changes := results.Changes.Elements()
	if len(changes) != 1 {
		t.Fatalf("got %d changes, want 1", len(changes))
	}
	if got, want := changes[0].Value.From.String(), "random_pet.a"; got != want {
		t.Errorf("moved from %q, want %q", got, want)
	}
}

func TestApplyMoves_Blocked(t *testing.T) {
	cfg := loadConfig(t, `
resource "random_pet" "b" {}

moved {
  from = random_pet.a
  to   = random_pet.b
}
`)
	stmts := xrefactoring.FindMoveStatements(cfg)
	st := stateWith(t, "random_pet.a", "random_pet.b")
	results := xrefactoring.ApplyMoves(stmts, st)

	if got := stateAddrs(st); len(got) != 2 {
		t.Errorf("state addresses = %v, want both retained", got)
	}
	if results.Blocked.Len() != 1 {
		t.Fatalf("got %d blocked moves, want 1", results.Blocked.Len())
	}
	blocked := results.Blocked.Elements()[0].Value
	if got, want := blocked.Actual.String(), "random_pet.a"; got != want {
		t.Errorf("blocked actual = %q, want %q", got, want)
	}
	if got, want := blocked.Wanted.String(), "random_pet.b"; got != want {
		t.Errorf("blocked wanted = %q, want %q", got, want)
	}
}

func TestValidateMoveStatementGraph_Cycle(t *testing.T) {
	cfg := loadConfig(t, `
resource "random_pet" "a" {}
resource "random_pet" "b" {}

moved {
  from = random_pet.a
  to   = random_pet.b
}

moved {
  from = random_pet.b
  to   = random_pet.a
}
`)
	stmts := xrefactoring.FindMoveStatements(cfg)
	err := xrefactoring.ValidateMoveStatementGraph(stmts)
	if err == nil {
		t.Fatal("cyclic move statements validated without error")
	}

	// The reason the check exists: ApplyMoves declines to move anything at all
	// rather than reporting the cycle itself.
	st := stateWith(t, "random_pet.a")
	xrefactoring.ApplyMoves(stmts, st)
	if got := stateAddrs(st); len(got) != 1 || got[0] != "random_pet.a" {
		t.Errorf("state addresses = %v, want [random_pet.a] (unmoved)", got)
	}
}

func TestImpliedMoveStatements_CountAdded(t *testing.T) {
	cfg := loadConfig(t, `
resource "random_pet" "a" {
  count = 1
}
`)
	explicit := xrefactoring.FindMoveStatements(cfg)
	if len(explicit) != 0 {
		t.Fatalf("got %d explicit statements, want 0", len(explicit))
	}

	st := stateWith(t, "random_pet.a")
	implied := xrefactoring.ImpliedMoveStatements(cfg, st, explicit)
	if len(implied) != 1 {
		t.Fatalf("got %d implied statements, want 1", len(implied))
	}
	if !implied[0].Implied {
		t.Error("statement is not marked Implied")
	}

	xrefactoring.ApplyMoves(implied, st)
	if got := stateAddrs(st); len(got) != 1 || got[0] != "random_pet.a[0]" {
		t.Errorf("state addresses = %v, want [random_pet.a[0]]", got)
	}
}

// lastContaining reports which statement, by index, is the last to contain addr —
// the precedence OpenTofu's orphan planning applies — or -1.
func lastContaining(t *testing.T, stmts []*xrefactoring.RemoveStatement, addr string) int {
	t.Helper()
	inst, err := xaddrs.ParseAbsResourceInstance(addr)
	if err != nil {
		t.Fatalf("parse %q: %v", addr, err)
	}
	found := -1
	for i, s := range stmts {
		if s.From.TargetContains(inst) {
			found = i
		}
	}
	return found
}

func TestFindRemoveStatements_ResourceAndModule(t *testing.T) {
	cfg := loadConfig(t, `
removed {
  from = random_pet.gone
  lifecycle {
    destroy = false
  }
}

removed {
  from = module.old
  lifecycle {
    destroy = true
  }
}
`)
	stmts, err := xrefactoring.FindRemoveStatements(cfg)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(stmts) != 2 {
		t.Fatalf("got %d statements, want 2", len(stmts))
	}

	cases := []struct {
		addr    string
		want    int
		destroy bool
	}{
		{"random_pet.gone", 0, false},
		{`random_pet.gone["k"]`, 0, false},
		{"module.old.random_pet.x", 1, true},
		{"module.old[1].module.inner.random_pet.x[0]", 1, true},
		{"random_pet.kept", -1, false},
		{"module.other.random_pet.gone", -1, false},
	}
	for _, c := range cases {
		got := lastContaining(t, stmts, c.addr)
		if got != c.want {
			t.Errorf("%s: contained by statement %d, want %d", c.addr, got, c.want)
			continue
		}
		if got >= 0 && stmts[got].Destroy != c.destroy {
			t.Errorf("%s: destroy = %t, want %t", c.addr, stmts[got].Destroy, c.destroy)
		}
	}
}

func TestFindRemoveStatements_StillDeclared(t *testing.T) {
	cfg := loadConfig(t, `
resource "random_pet" "kept" {}

removed {
  from = random_pet.kept
}
`)
	if _, err := xrefactoring.FindRemoveStatements(cfg); err == nil {
		t.Fatal("a removed block naming a declared resource found without error")
	}
}
