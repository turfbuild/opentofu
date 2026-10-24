// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// StaticPaths is a downstream host's answer for the filesystem as a static
// evaluation sees it: the directory its filesystem functions resolve relative
// paths against (abspath with them), path.cwd, and how path.module spells a
// module's directory.
//
// OpenTofu runs in the configuration directory, so its static scope resolves
// against "." and answers path.cwd from os.Getwd. A host evaluating many
// configurations in one process, whose working directory is none of theirs,
// sets this so that statically-evaluated positions -- module source and
// version, backend blocks, provider for_each -- answer as its runtime
// evaluation does. Nil keeps upstream's behaviour.
type StaticPaths struct {
	// BaseDir is where relative paths in filesystem functions resolve, and
	// abspath with them (lang.Scope.AbsPathFromBaseDir).
	BaseDir string
	// Cwd answers path.cwd. Empty refuses path.cwd rather than answering with
	// the host process's working directory.
	Cwd string
	// Module spells a module's source directory for path.module. inPackage
	// reports whether the module lies inside a module package: called through
	// a non-local source, or through local sources from a module that was.
	// Nil answers with the source directory, as upstream does.
	Module func(sourceDir string, inPackage bool) string
}

// WithStaticPaths returns the call with paths set; nil restores upstream's
// behaviour. The paths, the call's rootPath, and whether each module lies
// inside a module package are carried to every module called beneath it
// (childStaticCall).
func (s StaticModuleCall) WithStaticPaths(paths *StaticPaths) StaticModuleCall {
	s.paths = paths
	return s
}

// childStaticCall is the static call for a module called from parent through
// source. Without StaticPaths on the parent's call it is upstream's: path.root
// is the root module's directory. With them, the child inherits the paths and
// the root call's rootPath, so path.root answers alike at every depth, and
// learns whether it lies inside a module package, as walking the built tree's
// EntersNewPackage up from it will report.
func childStaticCall(parent *Config, path addrs.Module, declRange hcl.Range, vars StaticModuleVariables, source addrs.ModuleSource, workspace string) StaticModuleCall {
	call := NewStaticModuleCall(path, declRange, vars, parent.Root.Module.SourceDir, workspace)
	pcall, ok := staticCallOf(parent)
	if !ok || pcall.paths == nil {
		return call
	}
	if rcall, ok := staticCallOf(parent.Root); ok {
		call.rootPath = rcall.rootPath
	}
	call.paths = pcall.paths
	call.inPackage = pcall.inPackage || moduleSourceAddrEntersNewPackage(source)
	return call
}

func staticCallOf(cfg *Config) (StaticModuleCall, bool) {
	if cfg == nil || cfg.Module == nil || cfg.Module.StaticEvaluator == nil {
		return StaticModuleCall{}, false
	}
	return cfg.Module.StaticEvaluator.call, true
}

// applyStaticPaths points a static scope's filesystem functions at the host's
// base directory.
func applyStaticPaths(scope *lang.Scope, paths *StaticPaths) {
	if paths == nil {
		return
	}
	scope.BaseDir = paths.BaseDir
	scope.AbsPathFromBaseDir = true
}

// staticPathAttr answers path.cwd and path.module from the host's StaticPaths;
// ok false leaves the attribute to upstream's code.
func staticPathAttr(eval *StaticEvaluator, name string, rng tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics, bool) {
	paths := eval.call.paths
	if paths == nil {
		return cty.NilVal, nil, false
	}
	var diags tfdiags.Diagnostics
	switch name {
	case "cwd":
		if paths.Cwd == "" {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  `Failed to get working directory`,
				Detail:   `The value for path.cwd cannot be determined here: the host evaluating this configuration was given no working directory.`,
				Subject:  rng.ToHCL().Ptr(),
			})
			return cty.DynamicVal, diags, true
		}
		return cty.StringVal(filepath.ToSlash(paths.Cwd)), diags, true
	case "module":
		if paths.Module == nil {
			return cty.NilVal, nil, false
		}
		return cty.StringVal(paths.Module(eval.cfg.SourceDir, eval.call.inPackage)), diags, true
	}
	return cty.NilVal, nil, false
}
