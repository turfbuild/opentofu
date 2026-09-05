// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package lang

import (
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// expr parses a bare HCL expression for the meta-argument tests.
func expr(t *testing.T, src string) hcl.Expression {
	t.Helper()
	e, diags := hclsyntax.ParseExpression([]byte(src), "test.hcl", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("parsing %q: %s", src, diags.Error())
	}
	return e
}

// scopeWith builds a scope whose var.* entries are the given values. It hands
// back the seam scope the checks take, so these cases run against the same
// evaluation surface an arbitrary Data-backed caller would.
func scopeWith(vals map[string]cty.Value) *EvalScope {
	s := NewScope()
	for k, v := range vals {
		s.SetVariable(k, v)
	}
	return s.EvalScope()
}

// keys renders the pair keys as strings for compact assertions.
func keys(pairs []ForEachPair) []string {
	out := make([]string, 0, len(pairs))
	for _, p := range pairs {
		if p.Key.Type() == cty.String {
			out = append(out, p.Key.AsString())
			continue
		}
		out = append(out, p.Key.GoString())
	}
	return out
}

func TestEvaluateForEach_Enumerates(t *testing.T) {
	cases := []struct {
		name string
		val  cty.Value
		want []string
	}{
		{
			name: "MapSortsKeys",
			val: cty.MapVal(map[string]cty.Value{
				"c": cty.StringVal("3"), "a": cty.StringVal("1"), "b": cty.StringVal("2"),
			}),
			want: []string{"a", "b", "c"},
		},
		{
			name: "ObjectSortsAttributes",
			val: cty.ObjectVal(map[string]cty.Value{
				"z": cty.NumberIntVal(1), "y": cty.StringVal("two"),
			}),
			want: []string{"y", "z"},
		},
		{
			name: "SetOfStringsKeyedByElement",
			val:  cty.SetVal([]cty.Value{cty.StringVal("b"), cty.StringVal("a")}),
			want: []string{"a", "b"},
		},
		{
			// A map whose values are still unknown is enumerable: the keys are
			// what identify the instances, and whatever reads each.value defers
			// on its own.
			name: "MapWithUnknownValues",
			val: cty.MapVal(map[string]cty.Value{
				"a": cty.UnknownVal(cty.String), "b": cty.StringVal("known"),
			}),
			want: []string{"a", "b"},
		},
		{
			name: "EmptyMapYieldsNothing",
			val:  cty.MapValEmpty(cty.String),
			want: []string{},
		},
		{
			// The shape `toset([])` produces: an empty set whose element type
			// could not be inferred. It declares zero instances; it is not a
			// type error.
			name: "EmptyDynamicSetYieldsNothing",
			val:  cty.SetValEmpty(cty.DynamicPseudoType),
			want: []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := scopeWith(map[string]cty.Value{"x": tc.val})
			pairs, deferred, err := EvaluateForEach(expr(t, "var.x"), s, false, true)
			if err != nil {
				t.Fatalf("want enumeration, got error: %v", err)
			}
			if deferred {
				t.Fatalf("want enumeration, got a deferral")
			}
			got := keys(pairs)
			if len(got) != len(tc.want) {
				t.Fatalf("want keys %v, got %v", tc.want, got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("want keys %v, got %v", tc.want, got)
				}
			}
		})
	}
}

func TestEvaluateForEach_Defers(t *testing.T) {
	cases := []struct {
		name string
		val  cty.Value
	}{
		{"WhollyUnknown", cty.UnknownVal(cty.Map(cty.String))},
		{"DynamicUnknown", cty.DynamicVal},
		{
			// The elements of a set are its instance keys, so one unknown
			// element makes every address unknowable.
			name: "SetWithUnknownElement",
			val:  cty.SetVal([]cty.Value{cty.StringVal("a"), cty.UnknownVal(cty.String)}),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := scopeWith(map[string]cty.Value{"x": tc.val})
			pairs, deferred, err := EvaluateForEach(expr(t, "var.x"), s, false, true)
			if err != nil {
				t.Fatalf("want a deferral, got error: %v", err)
			}
			if !deferred {
				t.Fatalf("want a deferral, got %d pairs", len(pairs))
			}
		})
	}

	t.Run("UnknownIsAnErrorWhenUnknownsAreNotAllowed", func(t *testing.T) {
		s := scopeWith(map[string]cty.Value{"x": cty.UnknownVal(cty.Map(cty.String))})
		_, deferred, err := EvaluateForEach(expr(t, "var.x"), s, true, false)
		if deferred {
			t.Fatalf("allowUnknown=false must never defer")
		}
		if err == nil {
			t.Fatalf("want an error for an unknown for_each")
		}
	})
}

func TestEvaluateForEach_Rejects(t *testing.T) {
	cases := []struct {
		name    string
		val     cty.Value
		wantSub string
	}{
		{
			// A null element cannot become an instance key. Reaching the
			// enumerator with one used to panic.
			name:    "SetWithNullElement",
			val:     cty.SetVal([]cty.Value{cty.StringVal("a"), cty.NullVal(cty.String)}),
			wantSub: "must not contain null values",
		},
		{"Null", cty.NullVal(cty.Map(cty.String)), "null"},
		{"NonCollection", cty.StringVal("nope"), "must be a map, or set of strings"},
		{"Tuple", cty.TupleVal([]cty.Value{cty.StringVal("a")}), "must be a map, or set of strings"},
		{"SetOfNumbers", cty.SetVal([]cty.Value{cty.NumberIntVal(1)}), "sets of strings"},
		{"Sensitive", MarkSensitive(cty.MapVal(map[string]cty.Value{"a": cty.StringVal("1")})), "Sensitive values"},
		{"Ephemeral", MarkEphemeral(cty.MapVal(map[string]cty.Value{"a": cty.StringVal("1")})), "Ephemeral values"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := scopeWith(map[string]cty.Value{"x": tc.val})
			_, deferred, err := EvaluateForEach(expr(t, "var.x"), s, false, true)
			if deferred {
				t.Fatalf("want a permanent error, got a deferral")
			}
			if err == nil {
				t.Fatalf("want a permanent error, got success")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("want an error mentioning %q, got: %v", tc.wantSub, err)
			}
		})
	}
}

func TestEvaluateForEach_AllowTuple(t *testing.T) {
	s := scopeWith(map[string]cty.Value{
		"x": cty.TupleVal([]cty.Value{cty.StringVal("first"), cty.StringVal("second")}),
	})
	pairs, deferred, err := EvaluateForEach(expr(t, "var.x"), s, true, false)
	if err != nil || deferred {
		t.Fatalf("a tuple must enumerate when tuples are allowed; deferred=%v err=%v", deferred, err)
	}
	if len(pairs) != 2 {
		t.Fatalf("want 2 pairs, got %d", len(pairs))
	}
	// Index-keyed and in index order, which is what each.key resolves to.
	for i, want := range []string{"first", "second"} {
		idx, _ := pairs[i].Key.AsBigFloat().Int64()
		if int(idx) != i {
			t.Fatalf("pair %d: want index key %d, got %v", i, i, pairs[i].Key.GoString())
		}
		if pairs[i].Value.AsString() != want {
			t.Fatalf("pair %d: want value %q, got %q", i, want, pairs[i].Value.AsString())
		}
	}
}

func TestEvaluateCount(t *testing.T) {
	t.Run("Number", func(t *testing.T) {
		s := scopeWith(map[string]cty.Value{"x": cty.NumberIntVal(3)})
		n, deferred, err := EvaluateCount(expr(t, "var.x"), s)
		if err != nil || deferred || n != 3 {
			t.Fatalf("want 3, got n=%d deferred=%v err=%v", n, deferred, err)
		}
	})

	t.Run("NumericStringConverts", func(t *testing.T) {
		// How a JSON-shaped caller serializes a count. HCL's own conversion
		// applies because the evaluator is asked for a cty.Number.
		s := scopeWith(map[string]cty.Value{"x": cty.StringVal("2")})
		n, deferred, err := EvaluateCount(expr(t, "var.x"), s)
		if err != nil || deferred || n != 2 {
			t.Fatalf(`want "2" to read as 2, got n=%d deferred=%v err=%v`, n, deferred, err)
		}
	})

	t.Run("SensitiveIsAllowed", func(t *testing.T) {
		s := scopeWith(map[string]cty.Value{"x": MarkSensitive(cty.NumberIntVal(2))})
		n, deferred, err := EvaluateCount(expr(t, "var.x"), s)
		if err != nil || deferred || n != 2 {
			t.Fatalf("a sensitive count is permitted; got n=%d deferred=%v err=%v", n, deferred, err)
		}
	})

	t.Run("UnknownDefers", func(t *testing.T) {
		s := scopeWith(map[string]cty.Value{"x": cty.UnknownVal(cty.Number)})
		_, deferred, err := EvaluateCount(expr(t, "var.x"), s)
		if err != nil || !deferred {
			t.Fatalf("want a deferral, got deferred=%v err=%v", deferred, err)
		}
	})

	for _, tc := range []struct {
		name string
		val  cty.Value
	}{
		{"Null", cty.NullVal(cty.Number)},
		{"Negative", cty.NumberIntVal(-1)},
		{"NonNumericString", cty.StringVal("abc")},
		{"Bool", cty.True},
		{"Ephemeral", MarkEphemeral(cty.NumberIntVal(2))},
	} {
		t.Run("Rejects"+tc.name, func(t *testing.T) {
			s := scopeWith(map[string]cty.Value{"x": tc.val})
			_, deferred, err := EvaluateCount(expr(t, "var.x"), s)
			if deferred {
				t.Fatalf("want a permanent error, got a deferral")
			}
			if err == nil {
				t.Fatalf("want a permanent error, got success")
			}
		})
	}
}

func TestEvaluateEnabled(t *testing.T) {
	t.Run("TrueAndFalse", func(t *testing.T) {
		for _, want := range []bool{true, false} {
			s := scopeWith(map[string]cty.Value{"x": cty.BoolVal(want)})
			got, deferred, err := EvaluateEnabled(expr(t, "var.x"), s)
			if err != nil || deferred || got != want {
				t.Fatalf("want %v, got %v deferred=%v err=%v", want, got, deferred, err)
			}
		}
	})

	t.Run("UnknownDefers", func(t *testing.T) {
		s := scopeWith(map[string]cty.Value{"x": cty.UnknownVal(cty.Bool)})
		_, deferred, err := EvaluateEnabled(expr(t, "var.x"), s)
		if err != nil || !deferred {
			t.Fatalf("want a deferral, got deferred=%v err=%v", deferred, err)
		}
	})

	for _, tc := range []struct {
		name string
		val  cty.Value
	}{
		{"Null", cty.NullVal(cty.Bool)},
		{"NonBool", cty.NumberIntVal(3)},
		{"Sensitive", MarkSensitive(cty.True)},
		{"Ephemeral", MarkEphemeral(cty.True)},
	} {
		t.Run("Rejects"+tc.name, func(t *testing.T) {
			s := scopeWith(map[string]cty.Value{"x": tc.val})
			_, deferred, err := EvaluateEnabled(expr(t, "var.x"), s)
			if deferred {
				t.Fatalf("want a permanent error, got a deferral")
			}
			if err == nil {
				t.Fatalf("want a permanent error, got success")
			}
		})
	}
}
