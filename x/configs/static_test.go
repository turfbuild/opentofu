// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	otfconfigs "github.com/opentofu/opentofu/internal/configs"
	"github.com/zclconf/go-cty/cty"
)

// staticConfig is a root module whose module block reaches every symbol the
// static call carries: a variable, path.root, and terraform.workspace.
const staticConfig = `
variable "env" {
  type = string
}

module "child" {
  source = "./child-${var.env}"
}

output "root_path" { value = path.root }
output "workspace" { value = terraform.workspace }
`

// TestParseModuleResolvesStaticVariables covers the whole reason a
// StaticModuleCall is a required parameter: a var.* reference in a
// statically-evaluated position is resolved by the call's vars func, and with
// no vars func at all the reference must produce a diagnostic rather than
// dereferencing nil.
func TestParseModuleResolvesStaticVariables(t *testing.T) {
	dir := writeFiles(t, map[string]string{"main.tf": staticConfig})

	t.Run("with values", func(t *testing.T) {
		mod, err := ParseModule(dir, RootModuleCall(dir, "staging", func(v *Variable) (cty.Value, hcl.Diagnostics) {
			if v.Name != "env" {
				t.Errorf("asked for unexpected variable %q", v.Name)
			}
			return cty.StringVal("prod"), nil
		}))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		got := mod.ModuleCalls["child"].SourceAddrRaw
		if want := "./child-prod"; got != want {
			t.Errorf("module source = %q, want %q", got, want)
		}
	})

	// Without this guard the nil vars func panics inside the static scope,
	// taking the whole host process with it.
	t.Run("without values", func(t *testing.T) {
		_, err := ParseModule(dir, RootModuleCall(dir, "staging", nil))
		if err == nil {
			t.Fatal("expected a diagnostic, got a clean parse")
		}
		if !strings.Contains(err.Error(), "var.env") {
			t.Errorf("diagnostic does not name the variable: %v", err)
		}
	})
}

// TestModuleVersionFromVariable covers the second statically-evaluated field
// of a module call. It decodes through a different path than the source
// (decodeStaticVersion, not the source-address parser), so it needs its own
// witness — and a registry source parses without any network, since nothing is
// fetched at parse time.
func TestModuleVersionFromVariable(t *testing.T) {
	dir := writeFiles(t, map[string]string{"main.tf": `
variable "v" {
  type = string
}

module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = var.v
}
`})
	mod, err := ParseModule(dir, RootModuleCall(dir, "", func(*Variable) (cty.Value, hcl.Diagnostics) {
		return cty.StringVal("5.1.2"), nil
	}))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := mod.ModuleCalls["vpc"].Version.Required.String()
	if want := "5.1.2"; got != want {
		t.Errorf("module version constraint = %q, want %q", got, want)
	}
}

// TestRootModuleCallCarriesPathAndWorkspace pins the two symbols that resolve
// to the empty string under a zero call — silently, which is worse than the
// panic because the configuration still loads.
func TestRootModuleCallCarriesPathAndWorkspace(t *testing.T) {
	dir := writeFiles(t, map[string]string{"main.tf": staticConfig})
	mod, err := ParseModule(dir, RootModuleCall(dir, "staging", func(*Variable) (cty.Value, hcl.Diagnostics) {
		return cty.StringVal("prod"), nil
	}))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	eval := mod.StaticEvaluator
	for _, tc := range []struct {
		output string
		want   string
	}{
		{"root_path", dir},
		{"workspace", "staging"},
	} {
		val, diags := eval.Evaluate(context.Background(), mod.Outputs[tc.output].Expr, otfconfigs.StaticIdentifier{Subject: tc.output})
		if diags.HasErrors() {
			t.Fatalf("evaluating %s: %s", tc.output, diags.Error())
		}
		if got := val.AsString(); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.output, got, tc.want)
		}
	}
}
