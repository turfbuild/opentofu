package configs

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/getproviders"
)

// TestActionProviderRequirements checks that an action's provider is a
// requirement of its module, as a resource's is: an action-only provider,
// implied or named, is required with no constraint, a required_providers entry
// keeps its constraint, and a built-in one is listed like terraform_data's.
func TestActionProviderRequirements(t *testing.T) {
	parser := testParser(map[string]string{
		"mod/main.tf": `
terraform {
  required_providers {
    mock = {
      source  = "example.com/turf/tfcoremock"
      version = "~> 0.6.0"
    }
  }
}

action "mock_noop" "constrained" {}

action "local_command" "implied" {
  config {
    command = "true"
  }
}

action "terraform_noop" "builtin" {}
`,
	})

	mod, diags := parser.LoadConfigDir("mod", NewStaticModuleCall(addrs.RootModule, hcl.Range{},
		func(v *Variable) (cty.Value, hcl.Diagnostics) { return v.Default, nil }, "<testing>", ""))
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags)
	}
	cfg, diags := BuildConfig(t.Context(), mod, nil)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags)
	}

	got, _, diags := cfg.ProviderRequirements()
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags)
	}
	want := getproviders.Requirements{
		addrs.MustParseProviderSourceString("example.com/turf/tfcoremock"): getproviders.MustParseVersionConstraints("~> 0.6.0"),
		addrs.NewDefaultProvider("local"):                                  nil,
		addrs.NewBuiltInProvider("terraform"):                              nil,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("wrong requirements\n%s", diff)
	}

	shallow, _ := cfg.ProviderRequirementsShallow()
	if diff := cmp.Diff(want, shallow); diff != "" {
		t.Errorf("wrong shallow requirements\n%s", diff)
	}
}
