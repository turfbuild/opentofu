// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package objchange

import (
	"testing"

	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/zclconf/go-cty/cty"
)

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
