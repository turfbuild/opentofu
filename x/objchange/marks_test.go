// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package objchange

import (
	"reflect"
	"testing"

	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/zclconf/go-cty/cty"
)

// TestSensitiveAsBool exercises the projection that backs SensitiveValues: a
// mark-carrying value becomes the OpenTofu `sensitive_values` shape (true at
// sensitive paths, non-sensitive entries omitted, nil when nothing is
// sensitive). Marks are applied directly with the same value ValueMarks uses.
func TestSensitiveAsBool(t *testing.T) {
	sens := func(v cty.Value) cty.Value { return v.Mark(marks.Sensitive) }

	tests := []struct {
		name string
		val  cty.Value
		want any
	}{
		{
			name: "nothing sensitive yields nil",
			val: cty.ObjectVal(map[string]cty.Value{
				"id": cty.StringVal("x"),
			}),
			want: nil,
		},
		{
			name: "single sensitive attribute",
			val: cty.ObjectVal(map[string]cty.Value{
				"id":     cty.StringVal("x"),
				"result": sens(cty.StringVal("secret")),
			}),
			want: map[string]any{"result": true},
		},
		{
			name: "sensitive nested object is true at the whole subtree",
			val: cty.ObjectVal(map[string]cty.Value{
				"creds": sens(cty.ObjectVal(map[string]cty.Value{
					"user": cty.StringVal("u"),
				})),
			}),
			want: map[string]any{"creds": true},
		},
		{
			name: "sensitive element inside a list",
			val: cty.ObjectVal(map[string]cty.Value{
				"items": cty.TupleVal([]cty.Value{
					cty.StringVal("public"),
					sens(cty.StringVal("private")),
				}),
			}),
			want: map[string]any{"items": []any{nil, true}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sensitiveAsBool(tt.val)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %#v\nwant %#v", got, tt.want)
			}
		})
	}
}

// TestSensitivePaths verifies the persistable-marks extractor: it returns the
// Sensitive path marks carried on a value (the subset OpenTofu stores as
// AttrSensitivePaths / `sensitive_attributes`), drops non-Sensitive marks such
// as Ephemeral (which must never reach state), and re-applies losslessly via
// MarkWithPaths.
func TestSensitivePaths(t *testing.T) {
	t.Run("no marks yields nil", func(t *testing.T) {
		v := cty.ObjectVal(map[string]cty.Value{"id": cty.StringVal("x")})
		if got := SensitivePaths(v); got != nil {
			t.Fatalf("want nil, got %#v", got)
		}
	})

	t.Run("extracts sensitive path and round-trips via MarkWithPaths", func(t *testing.T) {
		v := cty.ObjectVal(map[string]cty.Value{
			"id":     cty.StringVal("x"),
			"result": cty.StringVal("secret").Mark(marks.Sensitive),
		})
		paths := SensitivePaths(v)
		if len(paths) != 1 {
			t.Fatalf("want 1 path, got %d: %#v", len(paths), paths)
		}
		// Re-applying the extracted paths to the unmarked value restores the mark.
		unmarked, _ := v.UnmarkDeep()
		remarked := unmarked.MarkWithPaths(paths)
		if !remarked.GetAttr("result").HasMark(marks.Sensitive) {
			t.Errorf("result should be sensitive after MarkWithPaths")
		}
		if remarked.GetAttr("id").HasMark(marks.Sensitive) {
			t.Errorf("id should not be sensitive")
		}
	})

	t.Run("drops non-sensitive marks (ephemeral)", func(t *testing.T) {
		v := cty.ObjectVal(map[string]cty.Value{
			"token": cty.StringVal("t").Mark(marks.Ephemeral),
		})
		if got := SensitivePaths(v); got != nil {
			t.Fatalf("ephemeral-only value must not yield sensitive paths, got %#v", got)
		}
	})
}
