package configs

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
)

// TestActionProviderFQN checks that an action block resolves its provider the
// same way a resource block does: an explicit `provider` argument through this
// module's required_providers, an absent one through the type prefix it
// implies. Actions carry an optional provider argument, so the two must agree.
func TestActionProviderFQN(t *testing.T) {
	parser := testParser(map[string]string{
		"mod/main.tf": `
terraform {
  required_providers {
    mock = {
      source = "example.com/turf/tfcoremock"
    }
  }
}

provider "mock" {
  alias = "west"
}

action "mock_noop" "implied" {}

action "mock_noop" "explicit" {
  provider = mock.west
}

action "unrelated_noop" "defaulted" {}
`,
	})

	cfg, diags := parser.LoadConfigDir("mod", NewStaticModuleCall(addrs.RootModule, hcl.Range{},
		func(v *Variable) (cty.Value, hcl.Diagnostics) { return v.Default, nil }, "<testing>", ""))
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags)
	}

	cases := []struct {
		key       string
		wantFQN   string
		wantLocal string
		wantAlias string
	}{
		// The type prefix implies local name "mock", which required_providers
		// maps to a non-default registry and namespace.
		{"action.mock_noop.implied", "example.com/turf/tfcoremock", "mock", ""},
		// The explicit ref names the same local slot, keeping its alias.
		{"action.mock_noop.explicit", "example.com/turf/tfcoremock", "mock", "west"},
		// No requirements entry claims "unrelated", so it implies the default
		// registry's hashicorp namespace.
		{"action.unrelated_noop.defaulted", "registry.opentofu.org/hashicorp/unrelated", "unrelated", ""},
	}
	for _, tc := range cases {
		a, ok := cfg.Actions[tc.key]
		if !ok {
			t.Errorf("%s was not decoded", tc.key)
			continue
		}
		if got := a.Provider.String(); got != tc.wantFQN {
			t.Errorf("%s: Provider = %q, want %q", tc.key, got, tc.wantFQN)
		}
		local := a.ProviderConfigAddr()
		if local.LocalName != tc.wantLocal || local.Alias != tc.wantAlias {
			t.Errorf("%s: ProviderConfigAddr() = {%q, %q}, want {%q, %q}",
				tc.key, local.LocalName, local.Alias, tc.wantLocal, tc.wantAlias)
		}
	}
}
