package configs

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// TestActionTriggerNested covers the lifecycle.action_trigger form on a managed
// resource: events, actions and on_failure decode with the same vocabulary as
// the top-level form, and condition is retained as an expression and is nil
// when absent.
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
	if got := entrySpellings(gated.Actions); got != "action.mock_noop.hook action.mock_noop.gate" {
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
}

// TestActionTriggerRefusedOutsideManaged pins Terraform's refusal of a
// lifecycle.action_trigger on a data or an ephemeral resource, in its words
// (witnessed on Terraform 1.16.2): a trigger there has no managed-lifecycle
// event to fire on, and dropping it silently would skip what the author wrote.
func TestActionTriggerRefusedOutsideManaged(t *testing.T) {
	for _, kind := range []struct{ block, word string }{
		{`data "null_data_source" "probe"`, "data"},
		{`ephemeral "null_secret" "probe"`, "ephemeral"},
	} {
		t.Run(kind.word, func(t *testing.T) {
			_, diags := testTriggerModule(t, `
action "mock_noop" "hook" {}

`+kind.block+` {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.mock_noop.hook]
    }
  }
}
`)
			if !diags.HasErrors() {
				t.Fatal("expected an error diagnostic")
			}
			for _, want := range []string{
				"Invalid " + kind.word + " resource lifecycle nested block",
				`The lifecycle nested block "action_trigger" is defined only for managed resources ("resource" blocks), and is not valid for ` + kind.word + " resources.",
			} {
				if !strings.Contains(diags.Error(), want) {
					t.Errorf("diagnostics %q do not say %q", diags.Error(), want)
				}
			}
		})
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

// TestActionTriggerKeyedEntries pins the entry shapes the decode admits, in
// the nested form, the top-level form and a JSON file: an instance key that
// is a constant, count.index or each.key (Terraform's set). The shapes
// Terraform refuses only at validation — each.value, an attribute after the
// key — decode too; their check is the consumer's.
func TestActionTriggerKeyedEntries(t *testing.T) {
	const native = `
action "mock_noop" "fe" {
  for_each = toset(["a", "b"])
}
action "mock_noop" "counted" {
  count = 2
}

resource "null_resource" "web" {
  for_each = toset(["a", "b"])
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.mock_noop.fe["a"], action.mock_noop.fe[each.key], action.mock_noop.counted[0]]
    }
    action_trigger {
      events  = [after_update]
      actions = [action.mock_noop.fe[each.value], action.mock_noop.fe[each.key].config]
    }
  }
}

resource "null_resource" "db" {
  count = 2
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.mock_noop.counted[count.index]]
    }
  }
}

action_trigger "verify_each" {
  target  = null_resource.web
  events  = [after_create]
  actions = [action.mock_noop.fe[each.key]]
}
`
	const json = `{
  "action": {
    "mock_noop": {
      "fe": {"for_each": "${toset([\"a\", \"b\"])}"}
    }
  },
  "resource": {
    "null_resource": {
      "web": {
        "for_each": "${toset([\"a\", \"b\"])}",
        "lifecycle": [{
          "action_trigger": [{
            "events": ["after_create"],
            "actions": ["action.mock_noop.fe[\"a\"]", "action.mock_noop.fe[each.key]"]
          }]
        }]
      }
    }
  }
}`
	t.Run("native", func(t *testing.T) {
		mod, diags := testTriggerModule(t, native)
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags)
		}
		web := mod.ManagedResources["null_resource.web"].Managed.ActionTriggers
		if got, want := entrySpellings(web[0].Actions), `action.mock_noop.fe["a"] action.mock_noop.fe[each.key] action.mock_noop.counted[0]`; got != want {
			t.Errorf("web's first trigger actions = %q, want %q", got, want)
		}
		if got, want := entrySpellings(web[1].Actions), `action.mock_noop.fe[each.value] action.mock_noop.fe[each.key].config`; got != want {
			t.Errorf("web's second trigger actions = %q, want %q", got, want)
		}
		db := mod.ManagedResources["null_resource.db"].Managed.ActionTriggers
		if got, want := entrySpellings(db[0].Actions), `action.mock_noop.counted[count.index]`; got != want {
			t.Errorf("db's trigger actions = %q, want %q", got, want)
		}
		decl := mod.ActionTriggers["verify_each"]
		if decl == nil {
			t.Fatal("verify_each was not decoded")
		}
		if got, want := entrySpellings(decl.Actions), `action.mock_noop.fe[each.key]`; got != want {
			t.Errorf("verify_each actions = %q, want %q", got, want)
		}
	})
	t.Run("json", func(t *testing.T) {
		mod, diags := testTriggerModuleFile(t, "main.tf.json", json)
		if diags.HasErrors() {
			t.Fatalf("unexpected diagnostics: %s", diags)
		}
		web := mod.ManagedResources["null_resource.web"].Managed.ActionTriggers
		if len(web) != 1 {
			t.Fatalf("web has %d triggers, want 1", len(web))
		}
		if got, want := entrySpellings(web[0].Actions), `action.mock_noop.fe["a"] action.mock_noop.fe[each.key]`; got != want {
			t.Errorf("web's trigger actions = %q, want %q", got, want)
		}
		for _, e := range web[0].Actions {
			if _, ok := e.(hclsyntax.Expression); !ok {
				t.Errorf("entry %T is not native syntax", e)
			}
		}
	})
}

// TestActionTriggerEntryRefusals pins the one entry refusal the decode makes,
// in Terraform's words (witnessed on 1.16.2 at init, before validation): an
// entry that refers to something other than an action, count or each — by
// its root or by its index — on the nested and the top-level form alike.
func TestActionTriggerEntryRefusals(t *testing.T) {
	for _, tc := range []struct{ name, entry string }{
		{"a resource", `null_resource.other`},
		{"a local in the key", `action.mock_noop.fe[local.k]`},
		{"a variable in the key", `action.mock_noop.fe[var.k]`},
		{"no action at all", `"action.mock_noop.fe"`},
	} {
		for _, form := range []struct{ name, src string }{
			{"nested", `
resource "null_resource" "web" {
  for_each = toset(["a"])
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [ENTRY]
    }
  }
}`},
			{"top-level", `
resource "null_resource" "web" {}
action_trigger "t" {
  target  = null_resource.web
  events  = [after_create]
  actions = [ENTRY]
}`},
		} {
			t.Run(tc.name+"/"+form.name, func(t *testing.T) {
				_, diags := testTriggerModule(t, `
locals {
  k = "a"
}
variable "k" {
  default = "a"
}
action "mock_noop" "fe" {
  for_each = toset(["a"])
}
resource "null_resource" "other" {}
`+strings.ReplaceAll(form.src, "ENTRY", tc.entry))
				for _, want := range []string{
					"Invalid action argument inside action_triggers",
					"action_triggers.actions must only refer to actions in the current module, count.index, or each.key.",
				} {
					if !strings.Contains(diags.Error(), want) {
						t.Errorf("diagnostics %q do not say %q", diags.Error(), want)
					}
				}
			})
		}
	}
}

// entrySpellings renders actions-list entries in the shapes the decode
// admits — a traversal, or an index expression over one whose key is a
// traversal, and an attribute after either — space-separated, for a one-line
// comparison.
func entrySpellings(exprs []hcl.Expression) string {
	var spell func(hcl.Expression) string
	spell = func(e hcl.Expression) string {
		switch e := e.(type) {
		case *hclsyntax.ScopeTraversalExpr:
			return string(hclwrite.TokensForTraversal(e.Traversal).Bytes())
		case *hclsyntax.IndexExpr:
			return spell(e.Collection) + "[" + spell(e.Key) + "]"
		case *hclsyntax.RelativeTraversalExpr:
			return spell(e.Source) + string(hclwrite.TokensForTraversal(e.Traversal).Bytes())
		}
		return fmt.Sprintf("<%T>", e)
	}
	out := make([]string, 0, len(exprs))
	for _, e := range exprs {
		out = append(out, spell(e))
	}
	return strings.Join(out, " ")
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
