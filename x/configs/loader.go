// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"context"
	"os"
	"path/filepath"
	"sort"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
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

// LoadVariablesFile loads variable values from a .tfvars file.
// Returns a map of variable name to the parsed HCL expression.
func LoadVariablesFile(path string) (map[string]Expression, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Parse as HCL using hclparse
	parser := hclparse.NewParser()
	file, diags := parser.ParseHCL(content, path)
	if diags.HasErrors() {
		return nil, diagsToError(diags)
	}

	// Extract variable assignments from the file body
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

// AutoLoadVariablesFiles finds and loads all auto-loaded .tfvars files in a directory.
// This includes terraform.tfvars and *.auto.tfvars files.
func AutoLoadVariablesFiles(dir string) (map[string]Expression, error) {
	result := make(map[string]Expression)

	// Load terraform.tfvars if it exists
	tfvarsPath := filepath.Join(dir, "terraform.tfvars")
	if _, err := os.Stat(tfvarsPath); err == nil {
		vars, err := LoadVariablesFile(tfvarsPath)
		if err != nil {
			return nil, err
		}
		for k, v := range vars {
			result[k] = v
		}
	}

	// Load *.auto.tfvars files in alphabetical order (Terraform convention)
	matches, err := filepath.Glob(filepath.Join(dir, "*.auto.tfvars"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	for _, path := range matches {
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
