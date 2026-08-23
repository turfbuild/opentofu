// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	hcljson "github.com/hashicorp/hcl/v2/json"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configload"
	"github.com/spf13/afero"
)

// Loader wraps OpenTofu's config loader for loading complete module trees.
type Loader struct {
	loader *configload.Loader
}

// NewLoader creates a new configuration loader.
// modulesDir is where downloaded modules will be stored (typically ".terraform/modules").
func NewLoader(modulesDir string) (*Loader, error) {
	l, err := configload.NewLoader(&configload.Config{
		ModulesDir: modulesDir,
	})
	if err != nil {
		return nil, err
	}
	return &Loader{loader: l}, nil
}

// RefreshModules re-reads the module manifest from disk.
// Call this after module installation to pick up newly installed modules.
func (l *Loader) RefreshModules() error {
	return l.loader.RefreshModules()
}

// InternalLoader returns the underlying configload.Loader.
// This is needed by the Installer to construct the ModuleInstaller.
func (l *Loader) InternalLoader() *configload.Loader {
	return l.loader
}

// LoadConfig loads a complete configuration tree from a directory.
// This resolves module calls and builds the full Config tree.
func (l *Loader) LoadConfig(ctx context.Context, dir string) (*Config, error) {
	cfg, diags := l.loader.LoadConfig(ctx, dir, configs.StaticModuleCall{})
	if diags.HasErrors() {
		return nil, diagsToError(diags)
	}
	return cfg, nil
}

// ParseModule parses a single module directory without resolving child modules.
// This is faster when you only need the root module's configuration.
func ParseModule(dir string) (*Module, error) {
	parser := configs.NewParser(afero.NewOsFs())
	mod, diags := parser.LoadConfigDir(dir, configs.StaticModuleCall{})
	if diags.HasErrors() {
		return nil, diagsToError(diags)
	}
	return mod, nil
}

// LoadVariablesFile loads variable values from a .tfvars or .tfvars.json file
// (or any file passed to -var-file, which follows the same rules). Returns a
// map of variable name to the parsed HCL expression. Syntax selection matches
// OpenTofu's addVarsFromFile: a ".json" suffix parses as HCL-JSON, a ".tfvars"
// suffix as native syntax, and an ambiguous name (process substitution, say)
// sniffs for a leading "{". A `variable "x" {}` block in the file is the
// classic declare-vs-assign mistake and gets the same dedicated error.
func LoadVariablesFile(path string) (map[string]Expression, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	extJSON := strings.HasSuffix(path, ".json")
	extTfvars := strings.HasSuffix(path, ".tfvars")
	detectJSON := !extJSON && !extTfvars && strings.HasPrefix(strings.TrimSpace(string(content)), "{")

	var file *hcl.File
	var diags hcl.Diagnostics
	if extJSON || detectJSON {
		file, diags = hcljson.Parse(content, path)
	} else {
		file, diags = hclsyntax.ParseConfig(content, path, hcl.Pos{Line: 1, Column: 1})
	}
	if diags.HasErrors() || file == nil || file.Body == nil {
		return nil, diagsToError(diags)
	}

	// Probe for `variable` blocks before the real decode: assigning values is
	// what a varfile is for, and JustAttributes' generic "blocks are not
	// allowed" error would bury the actual mistake.
	content2, _, _ := file.Body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{{Type: "variable", LabelNames: []string{"name"}}},
	})
	if len(content2.Blocks) > 0 {
		name := content2.Blocks[0].Labels[0]
		return nil, fmt.Errorf(
			"variable declaration in variables file %s: to declare variable %q, place the block in a .tf file; to set its value here, use the definition syntax: %s = <value>",
			path, name, name)
	}

	attrs, diags := file.Body.JustAttributes()
	if diags.HasErrors() {
		return nil, diagsToError(diags)
	}

	result := make(map[string]Expression, len(attrs))
	for name, attr := range attrs {
		result[name] = attr.Expr
	}
	return result, nil
}

// CollectAutoVarFiles returns the auto-loaded variables files of a
// configuration directory, in OpenTofu's precedence order (later files
// override earlier ones): terraform.tfvars, terraform.tfvars.json, then every
// *.auto.tfvars / *.auto.tfvars.json in one combined lexical pass — the two
// extensions interleave in name order, exactly as addVarsFromDir's single
// sorted directory listing produces them.
func CollectAutoVarFiles(dir string) ([]string, error) {
	var paths []string
	for _, name := range []string{"terraform.tfvars", "terraform.tfvars.json"} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			paths = append(paths, p)
		}
	}
	infos, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	// os.ReadDir sorts by filename, so filtering preserves lexical order.
	for _, info := range infos {
		name := info.Name()
		if strings.HasSuffix(name, ".auto.tfvars") || strings.HasSuffix(name, ".auto.tfvars.json") {
			paths = append(paths, filepath.Join(dir, name))
		}
	}
	return paths, nil
}

// AutoLoadVariablesFiles loads every auto-loaded variables file of dir
// (see CollectAutoVarFiles) into one merged map, later files overriding
// earlier ones. Callers that need per-file attribution (warnings naming the
// file, precedence layering) should use CollectAutoVarFiles + LoadVariablesFile
// directly.
func AutoLoadVariablesFiles(dir string) (map[string]Expression, error) {
	paths, err := CollectAutoVarFiles(dir)
	if err != nil {
		return nil, err
	}
	result := make(map[string]Expression)
	for _, path := range paths {
		vars, err := LoadVariablesFile(path)
		if err != nil {
			return nil, err
		}
		for k, v := range vars {
			result[k] = v
		}
	}
	return result, nil
}

// diagsToError converts HCL diagnostics to an error.
func diagsToError(diags hcl.Diagnostics) error {
	if !diags.HasErrors() {
		return nil
	}
	// Return just the first error for simplicity
	for _, diag := range diags {
		if diag.Severity == hcl.DiagError {
			return diag
		}
	}
	return nil
}
