// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

// unionOver composes the caller's view for these tests the way a downstream
// consumer would: the real OS filesystem as the base, an in-memory layer
// holding generated files at absolute paths, read-only on top.
func unionOver(t *testing.T, layer map[string]string) FS {
	t.Helper()
	mem := afero.NewMemMapFs()
	for path, content := range layer {
		if err := afero.WriteFile(mem, path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return afero.NewReadOnlyFs(afero.NewCopyOnWriteFs(afero.NewOsFs(), mem))
}

// TestNewLoaderFS_UnionLayerContributes proves the load-bearing composition:
// a loader over a union filesystem sees the real directory's files AND the
// memory layer's generated files as one module, and the directory listing
// merge is deterministic.
func TestNewLoaderFS_UnionLayerContributes(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"main.tf": `resource "null_resource" "real" {}`,
	})
	fs := unionOver(t, map[string]string{
		filepath.Join(dir, "_gen_extra.tf"): `resource "null_resource" "generated" {}`,
	})

	loader, err := NewLoaderFS(filepath.Join(dir, ".terraform", "modules"), fs)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := loader.LoadConfig(context.Background(), dir, RootModuleCall(dir, "", nil))
	if err != nil {
		t.Fatal(err)
	}

	var addrs []string
	for addr := range cfg.Module.ManagedResources {
		addrs = append(addrs, addr)
	}
	sort.Strings(addrs)
	want := "null_resource.generated,null_resource.real"
	if got := strings.Join(addrs, ","); got != want {
		t.Errorf("resources = %q, want %q", got, want)
	}
}

// TestNewLoaderFS_LayerWinsOnCollision pins the union semantics the caller's
// collision-refusal check must assume: on a name collision the memory layer
// shadows the real file.
func TestNewLoaderFS_LayerWinsOnCollision(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"main.tf": `resource "null_resource" "real" {}`,
	})
	fs := unionOver(t, map[string]string{
		filepath.Join(dir, "main.tf"): `resource "null_resource" "shadow" {}`,
	})

	mod, err := ParseModuleFS(fs, dir, RootModuleCall(dir, "", nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := mod.ManagedResources["null_resource.shadow"]; !ok {
		t.Errorf("layer file did not shadow the base file; resources: %v", mod.ManagedResources)
	}
	if _, ok := mod.ManagedResources["null_resource.real"]; ok {
		t.Errorf("shadowed base file still visible")
	}
}

// TestLoadConfigWithSnapshot_RoundTrip proves the seal-successor path: capture
// a snapshot of a union view (memory-layer file included), reload from the
// snapshot alone, and get the same module tree with the Dir string preserved
// verbatim — which is what keeps path.root reporting the real directory.
func TestLoadConfigWithSnapshot_RoundTrip(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"main.tf": `resource "null_resource" "real" {}`,
	})
	fs := unionOver(t, map[string]string{
		filepath.Join(dir, "_gen_extra.tf"): `resource "null_resource" "generated" {}`,
	})

	loader, err := NewLoaderFS(filepath.Join(dir, ".terraform", "modules"), fs)
	if err != nil {
		t.Fatal(err)
	}
	_, snap, err := loader.LoadConfigWithSnapshot(context.Background(), dir, RootModuleCall(dir, "", nil))
	if err != nil {
		t.Fatal(err)
	}

	root, ok := snap.Modules[""]
	if !ok {
		t.Fatalf("snapshot has no root module; modules: %v", snap.Modules)
	}
	if root.Dir != dir {
		t.Errorf("snapshot root Dir = %q, want the real directory %q", root.Dir, dir)
	}
	if _, ok := root.Files["_gen_extra.tf"]; !ok {
		t.Errorf("memory-layer file absent from snapshot; files: %v", keysOf(root.Files))
	}
	if _, ok := root.Files["main.tf"]; !ok {
		t.Errorf("real file absent from snapshot; files: %v", keysOf(root.Files))
	}

	// Reload purely from the snapshot: no filesystem involved.
	reloaded := NewLoaderFromSnapshot(snap)
	cfg, err := reloaded.LoadConfig(context.Background(), dir, RootModuleCall(dir, "", nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, addr := range []string{"null_resource.real", "null_resource.generated"} {
		if _, ok := cfg.Module.ManagedResources[addr]; !ok {
			t.Errorf("snapshot reload lost %s", addr)
		}
	}
	if cfg.Module.SourceDir != dir {
		t.Errorf("reloaded SourceDir = %q, want %q", cfg.Module.SourceDir, dir)
	}
}

func keysOf(m map[string][]byte) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
