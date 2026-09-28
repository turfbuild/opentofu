package jsonconfig

import (
	"context"
	"encoding/json"
	"path"
	"testing"

	"github.com/google/go-cmp/cmp"
	version "github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl/v2"
	"github.com/spf13/afero"

	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/plugins"
)

// TestMarshalActions checks a module's actions in the configuration
// representation, in the shape `terraform show -json` prints them (Terraform
// 1.16): address, type, name, provider_config_key and the count or for_each
// expression, for the root module and a child. An action-only provider is in
// provider_config. The child's key names the root's configuration it
// inherits, as a resource's does, where Terraform prints "module.child:local".
func TestMarshalActions(t *testing.T) {
	fs := afero.Afero{Fs: afero.NewMemMapFs()}
	files := map[string]string{
		"root/main.tf": `
action "local_command" "hi" {
  count = 2
  config {
    command = "echo"
  }
}

module "child" {
  source = "./child"
}
`,
		"root/child/main.tf": `
variable "keys" {
  default = ["p"]
}

action "local_command" "c" {
  for_each = toset(var.keys)
  config {
    command = "true"
  }
}
`,
	}
	for name, src := range files {
		if err := fs.MkdirAll(path.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := fs.WriteFile(name, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	parser := configs.NewParser(fs)
	root, diags := parser.LoadConfigDir("root")
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}
	cfg, diags := configs.BuildConfig(t.Context(), root, configs.RootModuleCallForTesting(), configs.ModuleWalkerFunc(
		func(_ context.Context, req *configs.ModuleRequest) (*configs.Module, *version.Version, hcl.Diagnostics) {
			mod, diags := parser.LoadConfigDir(path.Join("root", req.SourceAddr.String()))
			return mod, nil, diags
		}, parser.LoadSymbolFilesInDir))
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}

	raw, err := Marshal(cfg, &plugins.Schemas{})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		ProviderConfig map[string]any `json:"provider_config"`
		RootModule     struct {
			Actions     []any `json:"actions"`
			ModuleCalls map[string]struct {
				Module struct {
					Actions []any `json:"actions"`
				} `json:"module"`
			} `json:"module_calls"`
		} `json:"root_module"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}

	var want struct {
		ProviderConfig map[string]any
		Root, Child    []any
	}
	if err := json.Unmarshal([]byte(`{
  "ProviderConfig": {
    "local": {"name": "local", "full_name": "registry.opentofu.org/hashicorp/local"}
  },
  "Root": [
    {"address": "action.local_command.hi", "type": "local_command", "name": "hi",
     "provider_config_key": "local", "count_expression": {"constant_value": 2}}
  ],
  "Child": [
    {"address": "action.local_command.c", "type": "local_command", "name": "c",
     "provider_config_key": "local", "for_each_expression": {"references": ["var.keys"]}}
  ]
}`), &want); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want.ProviderConfig, got.ProviderConfig); diff != "" {
		t.Errorf("provider_config\n%s", diff)
	}
	if diff := cmp.Diff(want.Root, got.RootModule.Actions); diff != "" {
		t.Errorf("root_module.actions\n%s", diff)
	}
	if diff := cmp.Diff(want.Child, got.RootModule.ModuleCalls["child"].Module.Actions); diff != "" {
		t.Errorf("module_calls.child.module.actions\n%s", diff)
	}
}
