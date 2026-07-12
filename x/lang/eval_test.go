// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package lang

import (
	"reflect"
	"strings"
	"testing"

	"github.com/zclconf/go-cty/cty"
)

func TestEvalExpressionString_Templates(t *testing.T) {
	t.Run("MixedTemplateConcatenates", func(t *testing.T) {
		s := NewScope()
		s.SetResource("random_pet.a", cty.ObjectVal(map[string]cty.Value{
			"id": cty.StringVal("able-mongoose"),
		}))
		s.SetResource("random_pet.b", cty.ObjectVal(map[string]cty.Value{
			"id": cty.StringVal("brave-otter"),
		}))

		res, err := EvalExpressionString("${random_pet.a.id}-${random_pet.b.id}", s)
		if err != nil {
			t.Fatalf("mixed template should evaluate; got: %v", err)
		}
		if got := res.Value.AsString(); got != "able-mongoose-brave-otter" {
			t.Fatalf("want concatenated value, got %q", got)
		}
	})

	t.Run("SingleInterpolationPreservesNativeType", func(t *testing.T) {
		s := NewScope()
		s.SetVariable("items", cty.ListVal([]cty.Value{
			cty.StringVal("x"), cty.StringVal("y"),
		}))

		res, err := EvalExpressionString("${var.items}", s)
		if err != nil {
			t.Fatalf("single-interpolation template should evaluate; got: %v", err)
		}
		// A lone ${...} preserves the referenced value's native type rather than
		// rendering it as a string.
		if !res.Value.Type().IsListType() {
			t.Fatalf("want a list type preserved, got %s", res.Value.Type().FriendlyName())
		}
	})

	t.Run("UnresolvedRefPropagatesUnknown", func(t *testing.T) {
		s := NewScope()
		s.SetResource("random_pet.a", cty.ObjectVal(map[string]cty.Value{
			"id": cty.StringVal("able-mongoose"),
		}))
		// random_pet.b is absent — its interpolation resolves to unknown, so the
		// whole concatenated template is unknown.
		res, err := EvalExpressionString("${random_pet.a.id}-${random_pet.b.id}", s)
		if err != nil {
			t.Fatalf("unresolved ref should not error during eval; got: %v", err)
		}
		if res.IsKnown {
			t.Fatal("template with an unresolved ref should be unknown")
		}
	})
}

func TestFindUnknowns(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want []string
	}{
		{
			name: "wholly known map",
			in: map[string]any{
				"name": "foo",
				"size": int64(3),
				"tags": map[string]any{"env": "dev"},
				"list": []any{"a", "b"},
			},
			want: nil,
		},
		{
			name: "top level unknown leaf",
			in:   UnknownValue,
			want: []string{"<root>"},
		},
		{
			name: "unknown at top of map",
			in: map[string]any{
				"id":   UnknownValue,
				"name": "foo",
			},
			want: []string{"id"},
		},
		{
			name: "unknown inside nested map",
			in: map[string]any{
				"network": map[string]any{
					"cidr": UnknownValue,
					"name": "main",
				},
			},
			want: []string{"network.cidr"},
		},
		{
			name: "unknown inside list",
			in: map[string]any{
				"items": []any{"ok", UnknownValue, "ok"},
			},
			want: []string{"items[1]"},
		},
		{
			name: "unknown inside list of maps",
			in: map[string]any{
				"subnets": []any{
					map[string]any{"cidr": "10.0.0.0/24"},
					map[string]any{"cidr": UnknownValue},
				},
			},
			want: []string{"subnets[1].cidr"},
		},
		{
			name: "multiple unknowns sorted",
			in: map[string]any{
				"z": UnknownValue,
				"a": UnknownValue,
				"m": map[string]any{"x": UnknownValue},
			},
			want: []string{"a", "m.x", "z"},
		},
		{
			name: "non-unknown string ignored",
			in: map[string]any{
				"label": "__cty_known__",
			},
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FindUnknowns(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("FindUnknowns() = %#v; want %#v", got, tc.want)
			}
		})
	}
}

// TestEvalConfig_ProviderConfigureContract pins the exact ValidateConfig +
// EvalConfig + FindUnknowns behavior that tools/provider_configure.go relies on
// to turn config_init's ${...} provider config into known literals, the
// __cty_unknown__ sentinel (→ "configured_with_unknowns"), or a strict error.
func TestEvalConfig_ProviderConfigureContract(t *testing.T) {
	// kind_cluster.demo models a resource whose "endpoint" is unknown
	// (planned-but-unapplied in the open Draft) while "token" is a known secret
	// already in workspace state.
	scope := NewScope()
	scope.SetResource("kind_cluster.demo", cty.ObjectVal(map[string]cty.Value{
		"endpoint": cty.UnknownVal(cty.String),
		"token":    cty.StringVal("s3cr3t"),
	}))

	t.Run("KnownRefResolvesToRealLiteral", func(t *testing.T) {
		// A known ref (incl. a secret from state) resolves to its real value, not
		// a sentinel, so the provider is configured with the concrete value.
		out, err := EvalConfig(map[string]any{"host": "${kind_cluster.demo.token}"}, scope)
		if err != nil {
			t.Fatalf("EvalConfig: %v", err)
		}
		if out["host"] != "s3cr3t" {
			t.Fatalf("known secret ref should resolve to the real value, got %#v", out["host"])
		}
		if paths := FindUnknowns(out); paths != nil {
			t.Fatalf("known config should have no unknown paths, got %v", paths)
		}
	})

	t.Run("UnknownRefBecomesSentinelAtPath", func(t *testing.T) {
		// An unknown ref (at a scalar and inside a list) becomes __cty_unknown__,
		// and FindUnknowns reports the dotted/indexed paths — this is the
		// unknown_keys list and the "configured_with_unknowns" status.
		out, err := EvalConfig(map[string]any{
			"host":           "${kind_cluster.demo.endpoint}",
			"fail_on_create": []any{"${kind_cluster.demo.endpoint}"},
		}, scope)
		if err != nil {
			t.Fatalf("EvalConfig: %v", err)
		}
		if out["host"] != UnknownValue {
			t.Fatalf("unknown ref should become the sentinel, got %#v", out["host"])
		}
		list := out["fail_on_create"].([]any)
		if list[0] != UnknownValue {
			t.Fatalf("unknown list element should become the sentinel, got %#v", list[0])
		}
		want := []string{"fail_on_create[0]", "host"}
		if got := FindUnknowns(out); !reflect.DeepEqual(got, want) {
			t.Fatalf("FindUnknowns = %v, want %v", got, want)
		}
	})

	t.Run("NoInterpolationPassesThrough", func(t *testing.T) {
		in := map[string]any{"use_only_state": true, "region": "us-west-2"}
		out, err := EvalConfig(in, scope)
		if err != nil {
			t.Fatalf("EvalConfig: %v", err)
		}
		if !reflect.DeepEqual(out, in) {
			t.Fatalf("literal config should pass through unchanged, got %#v", out)
		}
		if paths := FindUnknowns(out); paths != nil {
			t.Fatalf("literal config should have no unknown paths, got %v", paths)
		}
	})

	t.Run("AbsentRefFailsStrictValidation", func(t *testing.T) {
		// The strict pre-flight rejects a reference to something in neither state
		// nor the Draft, naming the address, instead of silently producing a
		// sentinel.
		err := ValidateConfig(map[string]any{"host": "${kind_cluster.ghost.endpoint}"}, scope)
		if err == nil {
			t.Fatal("absent ref should fail strict validation")
		}
		if !strings.Contains(err.Error(), "kind_cluster.ghost") {
			t.Fatalf("error should name the unresolved address, got %v", err)
		}
	})
}

func TestFindUnresolvedUnknowns(t *testing.T) {
	cases := []struct {
		name     string
		original any
		eval     any
		want     []string
	}{
		{
			name:     "no unknowns either side",
			original: map[string]any{"id": "${a.id}"},
			eval:     map[string]any{"id": "v1"},
			want:     nil,
		},
		{
			name:     "expression resolved to unknown is reported",
			original: map[string]any{"prefix": "${a.id}"},
			eval:     map[string]any{"prefix": UnknownValue},
			want:     []string{"prefix"},
		},
		{
			name:     "preexisting unknown is suppressed",
			original: map[string]any{"name": UnknownValue},
			eval:     map[string]any{"name": UnknownValue},
			want:     nil,
		},
		{
			name: "mixed - only new unknowns reported",
			original: map[string]any{
				"baked":    UnknownValue,
				"expr":     "${a.id}",
				"resolved": "${a.sep}",
			},
			eval: map[string]any{
				"baked":    UnknownValue,
				"expr":     UnknownValue,
				"resolved": "_",
			},
			want: []string{"expr"},
		},
		{
			name: "nested preexisting unknown suppressed but new reported",
			original: map[string]any{
				"triggers": map[string]any{
					"name": UnknownValue,
					"role": "${a.role}",
				},
			},
			eval: map[string]any{
				"triggers": map[string]any{
					"name": UnknownValue,
					"role": UnknownValue,
				},
			},
			want: []string{"triggers.role"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FindUnresolvedUnknowns(tc.original, tc.eval)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("FindUnresolvedUnknowns() = %#v; want %#v", got, tc.want)
			}
		})
	}
}

// TestEvalConfigSensitivePaths verifies the second-stage (map-eval) config-flow
// sensitivity capture: a config attribute whose interpolation resolves to a
// Sensitive-marked scope value is reported at top-level-attribute granularity.
func TestEvalConfigSensitivePaths(t *testing.T) {
	s := NewScope()
	s.SetResource("random_password.pw", cty.ObjectVal(map[string]cty.Value{
		"result": MarkSensitive(cty.StringVal("s3cr3t")),
	}))
	s.SetResource("random_pet.plain", cty.ObjectVal(map[string]cty.Value{
		"id": cty.StringVal("good-dog"),
	}))

	t.Run("top-level scalar referencing a sensitive attr", func(t *testing.T) {
		cfg := map[string]any{
			"password": "${random_password.pw.result}",
			"name":     "${random_pet.plain.id}",
		}
		paths := EvalConfigSensitivePaths(cfg, s)
		if len(paths) != 1 {
			t.Fatalf("want 1 path, got %d: %#v", len(paths), paths)
		}
		if g, ok := paths[0].Path[0].(cty.GetAttrStep); !ok || g.Name != "password" {
			t.Errorf("want GetAttr{password}, got %#v", paths[0].Path)
		}
	})

	t.Run("sensitive nested under a map marks the whole top-level attribute", func(t *testing.T) {
		cfg := map[string]any{
			"keepers": map[string]any{"seed": "${random_password.pw.result}", "note": "plain"},
		}
		paths := EvalConfigSensitivePaths(cfg, s)
		if len(paths) != 1 {
			t.Fatalf("want 1 path, got %d: %#v", len(paths), paths)
		}
		p := paths[0].Path
		if len(p) != 1 {
			t.Fatalf("want top-level-granular (1-step) path, got %#v", p)
		}
		if g, ok := p[0].(cty.GetAttrStep); !ok || g.Name != "keepers" {
			t.Errorf("want GetAttr{keepers}, got %#v", p)
		}
	})

	t.Run("no sensitivity yields nil", func(t *testing.T) {
		cfg := map[string]any{"name": "${random_pet.plain.id}", "static": "literal"}
		if paths := EvalConfigSensitivePaths(cfg, s); len(paths) != 0 {
			t.Errorf("want no paths, got %#v", paths)
		}
	})
}
