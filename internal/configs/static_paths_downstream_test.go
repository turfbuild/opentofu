// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	version "github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/zclconf/go-cty/cty"
)

// TestStaticPaths_RootScope: with StaticPaths on the root call, the static
// scope's filesystem functions and abspath resolve against BaseDir rather than
// the process working directory, path.cwd is the host's, path.module is
// spelled by the host, and path.root stays the call's rootPath.
func TestStaticPaths_RootScope(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "marker.txt"), []byte("from-base"), 0o644); err != nil {
		t.Fatal(err)
	}
	parser := testParser(map[string]string{"eval.tf": `
locals {
	root   = path.root
	module = path.module
	cwd    = path.cwd
	read   = file("marker.txt")
	abs    = abspath("x")
}
`})
	file, diags := parser.LoadConfigFile("eval.tf")
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	var spelled []bool
	call := NewStaticModuleCall(nil, hcl.Range{}, nil, ".", "default").WithStaticPaths(&StaticPaths{
		BaseDir: base,
		Cwd:     "/launch",
		Module: func(sourceDir string, inPackage bool) string {
			spelled = append(spelled, inPackage)
			return "spelled:" + sourceDir
		},
	})
	mod, diags := NewModule([]*File{file}, nil, call, "/abs/root", SelectiveLoadAll)
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	for name, want := range map[string]string{
		"root":   ".",
		"module": "spelled:/abs/root",
		"cwd":    "/launch",
		"read":   "from-base",
		"abs":    filepath.ToSlash(filepath.Join(base, "x")),
	} {
		got, diags := mod.StaticEvaluator.Evaluate(t.Context(), mod.Locals[name].Expr, StaticIdentifier{Subject: "local." + name})
		if diags.HasErrors() {
			t.Errorf("local.%s: %s", name, diags.Error())
			continue
		}
		if s := filepath.ToSlash(got.AsString()); s != want {
			t.Errorf("local.%s = %q, want %q", name, s, want)
		}
	}
	if len(spelled) != 1 || spelled[0] {
		t.Errorf("path.module was spelled with inPackage %v; the root is outside every package", spelled)
	}
}

// TestStaticPaths_NoCwd: a host that sets StaticPaths without a working
// directory refuses path.cwd rather than answering with its own process's.
func TestStaticPaths_NoCwd(t *testing.T) {
	parser := testParser(map[string]string{"eval.tf": "locals {\n  cwd = path.cwd\n}\n"})
	file, diags := parser.LoadConfigFile("eval.tf")
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	call := NewStaticModuleCall(nil, hcl.Range{}, nil, ".", "default").WithStaticPaths(&StaticPaths{BaseDir: t.TempDir()})
	mod, _ := NewModule([]*File{file}, nil, call, "dir", SelectiveLoadAll)
	if _, diags := mod.StaticEvaluator.Evaluate(t.Context(), mod.Locals["cwd"].Expr, StaticIdentifier{Subject: "local.cwd"}); !diags.HasErrors() {
		t.Fatal("path.cwd answered with no working directory given")
	}
}

// inPackage is whether c lies inside a module package, read off the built
// tree: some module from c up to (not including) the root entered one.
func inPackage(c *Config) bool {
	for ; c != nil && c.Parent != nil; c = c.Parent {
		if c.EntersNewPackage() {
			return true
		}
	}
	return false
}

// TestStaticPaths_Children: the paths and the root call's rootPath reach every
// module beneath the root, and each child learns whether it lies inside a
// module package -- a non-local source enters one, and a local call from a
// module inside one stays in it -- so path.module is spelled as the runtime
// spells it. Without StaticPaths a child's path.root is the root's directory,
// upstream's answer.
func TestStaticPaths_Children(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, src string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	const report = "locals {\n  root = path.root\n  module = path.module\n}\n"
	write("root/main.tf", `
module "local" {
  source = "../shared"
}
module "remote" {
  source = "example.com/ns/name/sys"
  version = "1.0.0"
}
`)
	write("shared/main.tf", report)
	write("pkg/main.tf", report+"module \"inner\" {\n  source = \"./inner\"\n}\n")
	write("pkg/inner/main.tf", report)

	build := func(t *testing.T, call StaticModuleCall) *Config {
		t.Helper()
		parser := NewParser(nil)
		mod, diags := parser.LoadConfigDir(filepath.Join(dir, "root"), call)
		if diags.HasErrors() {
			t.Fatal(diags)
		}
		cfg, diags := BuildConfig(t.Context(), mod, ModuleWalkerFunc(
			func(_ context.Context, req *ModuleRequest) (*Module, *version.Version, hcl.Diagnostics) {
				var src string
				switch strings.Join(req.Path, ".") {
				case "local":
					src = "shared"
				case "remote":
					src = "pkg"
				case "remote.inner":
					src = "pkg/inner"
				}
				m, d := parser.LoadConfigDir(filepath.Join(dir, src), req.Call)
				return m, version.Must(version.NewVersion("1.0.0")), d
			}))
		if diags.HasErrors() {
			t.Fatal(diags)
		}
		return cfg
	}
	eval := func(t *testing.T, cfg *Config, name string) string {
		t.Helper()
		if cfg.Module.Locals[name] == nil {
			t.Fatalf("module %s has no local %s", cfg.Path, name)
		}
		v, diags := cfg.Module.StaticEvaluator.Evaluate(t.Context(), cfg.Module.Locals[name].Expr, StaticIdentifier{Subject: "local." + name})
		if diags.HasErrors() {
			t.Fatal(diags)
		}
		if v.Type() != cty.String {
			t.Fatalf("local.%s is %s", name, v.Type().FriendlyName())
		}
		return v.AsString()
	}

	t.Run("with StaticPaths", func(t *testing.T) {
		paths := &StaticPaths{
			BaseDir: filepath.Join(dir, "root"),
			Cwd:     dir,
			Module: func(sourceDir string, inPackage bool) string {
				if inPackage {
					return "pkg:" + filepath.Base(sourceDir)
				}
				return "local:" + filepath.Base(sourceDir)
			},
		}
		cfg := build(t, NewStaticModuleCall(nil, hcl.Range{}, nil, ".", "default").WithStaticPaths(paths))
		for path, want := range map[string]string{
			"local":        "local:shared",
			"remote":       "pkg:pkg",
			"remote.inner": "pkg:inner",
		} {
			c := cfg.Descendent(addrs.Module(strings.Split(path, ".")))
			if c == nil {
				t.Fatalf("no module %s", path)
			}
			if got := eval(t, c, "module"); got != want {
				t.Errorf("module.%s path.module = %q, want %q", path, got, want)
			}
			if got := eval(t, c, "root"); got != "." {
				t.Errorf("module.%s path.root = %q, want the root call's %q", path, got, ".")
			}
			if got := inPackage(c); got != (want[:4] == "pkg:") {
				t.Errorf("module.%s: the static call's package-ness disagrees with the built tree's (%v)", path, got)
			}
		}
	})

	t.Run("without StaticPaths", func(t *testing.T) {
		cfg := build(t, NewStaticModuleCall(nil, hcl.Range{}, nil, ".", "default"))
		if got, want := eval(t, cfg.Children["local"], "root"), filepath.Join(dir, "root"); got != want {
			t.Errorf("a child's path.root = %q, want upstream's root directory %q", got, want)
		}
	})
}
