// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
)

// TestModuleManifestRoundTrip: a directory without a manifest reads as empty,
// and a written manifest reads back record for record.
func TestModuleManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m, err := ReadModuleManifest(dir)
	if err != nil {
		t.Fatalf("reading a directory without a manifest: %v", err)
	}
	if len(m) != 0 {
		t.Fatalf("a directory without a manifest read %d records", len(m))
	}
	m[""] = ModuleRecord{Key: "", Dir: "/abs/root"}
	m["shared"] = ModuleRecord{Key: "shared", SourceAddr: "../shared", Dir: "/abs/shared"}
	if err := WriteModuleManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	read, err := ReadModuleManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := read["shared"]; got.SourceAddr != "../shared" || filepath.ToSlash(got.Dir) != "/abs/shared" {
		t.Errorf("record read back as %+v", got)
	}
}

// TestStaticPathsReachAModuleSource: a module source built from path.module
// is a local path when the host spells path.module as the runtime does, and
// an absolute one -- a separate package to the installer -- without
// StaticPaths, where path.module is the directory the loader was given.
func TestStaticPathsReachAModuleSource(t *testing.T) {
	dir := writeFiles(t, map[string]string{"main.tf": `
module "child" {
  source = "${path.module}/child"
}
`})
	noVars := func(*Variable) (cty.Value, hcl.Diagnostics) { return cty.NilVal, nil }

	plain, err := ParseModule(dir, RootModuleCall(dir, "default", noVars))
	if err != nil {
		t.Fatal(err)
	}
	if got := plain.ModuleCalls["child"].SourceAddr.String(); !strings.HasPrefix(got, "file://") {
		t.Errorf("without StaticPaths the source is %q, want the absolute directory as a file:// package", got)
	}

	call := RootModuleCall(".", "default", noVars).WithStaticPaths(&StaticPaths{
		BaseDir: dir,
		Cwd:     dir,
		Module:  func(string, bool) string { return "." },
	})
	spelled, err := ParseModule(dir, call)
	if err != nil {
		t.Fatal(err)
	}
	if got := spelled.ModuleCalls["child"].SourceAddr.String(); got != "./child" {
		t.Errorf("with StaticPaths the source is %q, want the local %q", got, "./child")
	}
}
