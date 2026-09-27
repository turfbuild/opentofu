package configs

import (
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
)

// TestActionTriggerNested covers the lifecycle.action_trigger form on a managed
// resource: events, actions and on_failure decode with the same vocabulary as
// the top-level form, condition is retained as an expression and is nil when
// absent, and a trigger inside a data resource's lifecycle is dropped rather
// than refused (a data resource has no managed-lifecycle events).
func TestActionTriggerNested(t *testing.T) {
	mod, diags := testTriggerModule(t, `
action "mock_noop" "gate" {}
action "mock_noop" "hook" {}

variable "gated" {
  type    = bool
  default = true
}

resource "null_resource" "web" {
  lifecycle {
    action_trigger {
      events     = [before_create, after_update]
      actions    = [action.mock_noop.gate]
      on_failure = continue
    }
    action_trigger {
      events    = [after_create]
      actions   = [action.mock_noop.hook, action.mock_noop.gate]
      condition = var.gated
    }
  }
}

data "null_data_source" "probe" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.mock_noop.hook]
    }
  }
}
`)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %s", diags)
	}

	web := mod.ManagedResources["null_resource.web"]
	if web == nil || web.Managed == nil {
		t.Fatal("null_resource.web was not decoded as a managed resource")
	}
	triggers := web.Managed.ActionTriggers
	if len(triggers) != 2 {
		t.Fatalf("ActionTriggers has %d entries, want 2: %v", len(triggers), triggers)
	}

	plain := triggers[0]
	if len(plain.Events) != 2 || plain.Events[0] != ActionBeforeCreate || plain.Events[1] != ActionAfterUpdate {
		t.Errorf("first trigger events = %v, want [before_create after_update]", plain.Events)
	}
	if plain.OnFailure != ActionOnFailureContinue {
		t.Errorf("first trigger on_failure = %v, want continue", plain.OnFailure)
	}
	if len(plain.Actions) != 1 {
		t.Errorf("first trigger actions = %v, want one", plain.Actions)
	}
	if plain.Condition != nil {
		t.Errorf("first trigger has an unexpected condition")
	}

	gated := triggers[1]
	if len(gated.Events) != 1 || gated.Events[0] != ActionAfterCreate {
		t.Errorf("second trigger events = %v, want [after_create]", gated.Events)
	}
	if gated.OnFailure != ActionOnFailureHalt {
		t.Errorf("second trigger on_failure = %v, want the halt default", gated.OnFailure)
	}
	if got := traversalNames(gated.Actions); got != "action.mock_noop.hook action.mock_noop.gate" {
		t.Errorf("second trigger actions = %q, want the two references in written order", got)
	}
	if gated.Condition == nil {
		t.Fatal("second trigger's condition was not retained")
	}
	condRef, condDiags := hcl.AbsTraversalForExpr(gated.Condition)
	if condDiags.HasErrors() {
		t.Fatalf("condition is not a plain reference: %s", condDiags)
	}
	if got := traversalNames([]hcl.Traversal{condRef}); got != "var.gated" {
		t.Errorf("condition references %q, want var.gated", got)
	}

	probe := mod.DataResources["data.null_data_source.probe"]
	if probe == nil {
		t.Fatal("data.null_data_source.probe was not decoded")
	}
	if probe.Managed != nil {
		t.Errorf("a data resource grew a managed block from its action_trigger")
	}
}

// TestActionTriggerNestedRefusals pins the decode-time refusals shared with the
// top-level form: an unknown event, an unknown on_failure keyword, and an
// argument the block does not accept.
func TestActionTriggerNestedRefusals(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			"unknown event",
			`events = [on_create]
			 actions = [action.mock_noop.gate]`,
			"Invalid action_trigger event",
		},
		{
			"unknown on_failure",
			`events = [after_create]
			 actions = [action.mock_noop.gate]
			 on_failure = retry`,
			"Invalid action_trigger on_failure",
		},
		{
			"unknown argument",
			`events = [after_create]
			 actions = [action.mock_noop.gate]
			 target = null_resource.web`,
			"Unsupported argument",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, diags := testTriggerModule(t, `
action "mock_noop" "gate" {}

resource "null_resource" "web" {
  lifecycle {
    action_trigger {
      `+tc.body+`
    }
  }
}
`)
			if !diags.HasErrors() {
				t.Fatal("expected an error diagnostic")
			}
			if !strings.Contains(diags.Error(), tc.want) {
				t.Errorf("diagnostics %q do not mention %q", diags.Error(), tc.want)
			}
		})
	}
}

// traversalNames renders traversals of attribute steps as dotted names,
// space-separated, for a one-line comparison.
func traversalNames(travs []hcl.Traversal) string {
	var out []string
	for _, trav := range travs {
		parts := []string{trav.RootName()}
		for _, step := range trav[1:] {
			if attr, ok := step.(hcl.TraverseAttr); ok {
				parts = append(parts, attr.Name)
			}
		}
		out = append(out, strings.Join(parts, "."))
	}
	return strings.Join(out, " ")
}
