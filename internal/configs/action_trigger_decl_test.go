package configs

import (
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
)

func testTriggerModule(t *testing.T, src string) (*Module, hcl.Diagnostics) {
	t.Helper()
	parser := testParser(map[string]string{"mod/main.tf": src})
	return parser.LoadConfigDir("mod", NewStaticModuleCall(addrs.RootModule, hcl.Range{},
		func(v *Variable) (cty.Value, hcl.Diagnostics) { return v.Default, nil }, "<testing>", ""))
}

// TestActionTriggerDecl covers the top-level address-targeted action_trigger
// block: root and module-child targets decode, the nested form's
// events/actions/on_failure vocabulary carries over unchanged, and condition
// is parsed and reserved.
func TestActionTriggerDecl(t *testing.T) {
	mod, diags := testTriggerModule(t, `
action "mock_noop" "gate" {}

resource "null_resource" "web" {}

action_trigger "root_gate" {
  target     = null_resource.web
  events     = [before_create, after_update]
  actions    = [action.mock_noop.gate]
  on_failure = continue
}

action_trigger "child_gate" {
  target  = module.network.aws_subnet.private
  events  = [before_destroy]
  actions = [action.mock_noop.gate]
}

action_trigger "conditional" {
  target    = null_resource.web
  events    = [after_create]
  actions   = [action.mock_noop.gate]
  condition = true
}
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags)
	}
	if len(mod.ActionTriggers) != 3 {
		t.Fatalf("ActionTriggers has %d entries, want 3: %v", len(mod.ActionTriggers), mod.ActionTriggers)
	}

	root := mod.ActionTriggers["root_gate"]
	if root == nil {
		t.Fatal("root_gate was not decoded")
	}
	if got := root.Target.Subject.String(); got != "null_resource.web" {
		t.Errorf("root_gate target = %q, want null_resource.web", got)
	}
	if len(root.Events) != 2 || root.Events[0] != ActionBeforeCreate || root.Events[1] != ActionAfterUpdate {
		t.Errorf("root_gate events = %v", root.Events)
	}
	if root.OnFailure != ActionOnFailureContinue {
		t.Errorf("root_gate on_failure = %v, want continue", root.OnFailure)
	}
	if len(root.Actions) != 1 {
		t.Errorf("root_gate actions = %v", root.Actions)
	}
	if root.Condition != nil {
		t.Errorf("root_gate has an unexpected condition")
	}

	child := mod.ActionTriggers["child_gate"]
	if child == nil {
		t.Fatal("child_gate was not decoded")
	}
	if got := child.Target.Subject.String(); got != "module.network.aws_subnet.private" {
		t.Errorf("child_gate target = %q, want the module-child address", got)
	}
	if child.OnFailure != ActionOnFailureHalt {
		t.Errorf("child_gate on_failure = %v, want the halt default", child.OnFailure)
	}

	cond := mod.ActionTriggers["conditional"]
	if cond == nil || cond.Condition == nil {
		t.Fatal("conditional's condition was not retained")
	}
}

// TestActionTriggerDeclRefusals pins the decode-time refusals: keyed instance
// targets, data-resource targets, bare module targets, duplicate names, and
// an unparseable address.
func TestActionTriggerDeclRefusals(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantErr string
	}{
		{
			"keyed instance target",
			`action_trigger "t" {
  target  = null_resource.web[2]
  events  = [before_create]
  actions = [action.mock_noop.gate]
}`,
			"whole resource",
		},
		{
			"data resource target",
			`action_trigger "t" {
  target  = data.null_data_source.d
  events  = [before_create]
  actions = [action.mock_noop.gate]
}`,
			"managed resource",
		},
		{
			"module target",
			`action_trigger "t" {
  target  = module.network
  events  = [before_create]
  actions = [action.mock_noop.gate]
}`,
			"managed resource address",
		},
		{
			"duplicate name",
			`resource "null_resource" "web" {}
action_trigger "t" {
  target  = null_resource.web
  events  = [before_create]
  actions = [action.mock_noop.gate]
}
action_trigger "t" {
  target  = null_resource.web
  events  = [after_create]
  actions = [action.mock_noop.gate]
}`,
			"Duplicate action_trigger",
		},
		{
			"invalid event",
			`action_trigger "t" {
  target  = null_resource.web
  events  = [on_fire]
  actions = [action.mock_noop.gate]
}`,
			"Invalid action_trigger event",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, diags := testTriggerModule(t, tc.src+"\n")
			if !diags.HasErrors() {
				t.Fatalf("want an error containing %q, got none", tc.wantErr)
			}
			if !strings.Contains(diags.Error(), tc.wantErr) {
				t.Errorf("diagnostics %q do not contain %q", diags.Error(), tc.wantErr)
			}
		})
	}
}
