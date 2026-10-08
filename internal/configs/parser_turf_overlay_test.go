// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
)

func loadOverlayTestDir(t *testing.T, files map[string]string) (*Module, hcl.Diagnostics) {
	t.Helper()
	parser := testParser(files)
	return parser.LoadConfigDir("mod", NewStaticModuleCall(addrs.RootModule, hcl.Range{},
		func(v *Variable) (cty.Value, hcl.Diagnostics) { return v.Default, nil }, "<testing>", ""))
}

// TestDirFilesAdmitsTurfExtension covers the Turf overlay extensions: a
// .turf.hcl file is native syntax, a .turf.json file is JSON, both merge into
// the module beside its .tf files, and the _override naming rule applies to
// them as to any configuration file. Files that merely end in .hcl or .json
// stay ignored.
func TestDirFilesAdmitsTurfExtension(t *testing.T) {
	mod, diags := loadOverlayTestDir(t, map[string]string{
		"mod/main.tf": `
resource "null_resource" "web" {}
`,
		"mod/gates.turf.hcl": `
action "mock_noop" "gate" {}

action_trigger "web_gate" {
  target  = null_resource.web
  events  = [before_create]
  actions = [action.mock_noop.gate]
}
`,
		"mod/more.turf.json": `{
  "action": {"mock_noop": {"json_gate": {}}},
  "action_trigger": {"json_web_gate": {
    "target": "null_resource.web",
    "events": ["after_create"],
    "actions": ["action.mock_noop.json_gate"]
  }}
}`,
		"mod/notes.hcl":    `this is not configuration`,
		"mod/data.json":    `{"neither": "is this"}`,
		"mod/x.tftest.hcl": `run "noop" {}`,
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags)
	}
	if mod.ManagedResources["null_resource.web"] == nil {
		t.Fatal("the .tf file's resource did not load")
	}
	for _, key := range []string{"action.mock_noop.gate", "action.mock_noop.json_gate"} {
		if mod.Actions[key] == nil {
			t.Errorf("%s did not load from its overlay file", key)
		}
	}
	gate := mod.ActionTriggers["web_gate"]
	if gate == nil {
		t.Fatal("web_gate did not load from gates.turf.hcl")
	}
	if got := gate.DeclRange.Filename; got != "mod/gates.turf.hcl" {
		t.Errorf("web_gate declared in %q, want mod/gates.turf.hcl", got)
	}
	if json := mod.ActionTriggers["json_web_gate"]; json == nil || json.DeclRange.Filename != "mod/more.turf.json" {
		t.Errorf("json_web_gate = %+v, want it declared in mod/more.turf.json", json)
	}
}

// TestTurfOverlayOverrideNaming: the _override naming rule classifies an
// overlay file exactly as it classifies a .tf file.
func TestTurfOverlayOverrideNaming(t *testing.T) {
	parser := testParser(map[string]string{
		"mod/main.tf":                 ``,
		"mod/gates.turf.hcl":          ``,
		"mod/gates_override.turf.hcl": ``,
		"mod/override.turf.json":      `{}`,
	})
	primary, override, _, diags := parser.dirFiles("mod", "")
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	if want := []string{"mod/gates.turf.hcl", "mod/main.tf"}; !equalStrings(primary, want) {
		t.Errorf("primary = %v, want %v", primary, want)
	}
	if want := []string{"mod/gates_override.turf.hcl", "mod/override.turf.json"}; !equalStrings(override, want) {
		t.Errorf("override = %v, want %v", override, want)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestTurfOverlayFileRecognition pins the extension rule itself: which names
// are overlays, and that an overlay-only directory is a module directory.
func TestTurfOverlayFileRecognition(t *testing.T) {
	for name, want := range map[string]bool{
		"gates.turf.hcl":          true,
		"gates.turf.json":         true,
		"gates_override.turf.hcl": true,
		"gates.tf":                false,
		"gates.hcl":               false,
		"gates.turf":              false,
		"gates.tftest.hcl":        false,
		"turf.hcl":                false,
	} {
		if got := IsTurfOverlayFile(name); got != want {
			t.Errorf("IsTurfOverlayFile(%q) = %v, want %v", name, got, want)
		}
	}
	parser := testParser(map[string]string{"only/gates.turf.hcl": `action "mock_noop" "gate" {}`})
	if !parser.IsConfigDir("only") {
		t.Error("a directory holding only an overlay file is not a configuration directory")
	}
}
