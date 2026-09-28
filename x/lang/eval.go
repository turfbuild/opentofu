// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package lang

import (
	"fmt"
	"sort"
	"strings"

	"github.com/zclconf/go-cty/cty"
)

// UnknownValue is the marker string emitted by CtyToGo for unknown cty
// leaves. Downstream consumers (provider plan/apply, JSON readers) treat
// this as "value not yet known" rather than a literal string.
const UnknownValue = "__cty_unknown__"

// ContainsInterpolation reports whether s contains an HCL interpolation
// (`${…}`) or directive (`%{…}`) sequence — i.e. HCL would parse it as a
// template with at least one dynamic part, whether the whole string is a single
// expression ("${var.foo}") or a mixed template ("${a}-${b}"). This is the gate
// the validate/eval/refs paths use to decide whether a string needs template
// parsing; plain strings (no sequence) pass through untouched.
//
// It does not account for escaping (`$${`): an escaped literal is reported as
// containing a sequence, which is harmless for its callers — hclsyntax.ParseTemplate
// resolves the escape to a literal (no refs, literal value), so validation/eval/refs
// all produce the correct result anyway.
func ContainsInterpolation(s string) bool {
	return strings.Contains(s, "${") || strings.Contains(s, "%{")
}

// FindUnknowns walks the given Go value (typically the output of
// EvalConfig) and returns the dotted paths of every leaf equal to
// UnknownValue. Map keys join with '.', slice indices use '[i]'.
// Returns nil when the value is wholly known. Paths are sorted for
// deterministic error messages.
func FindUnknowns(val any) []string {
	var paths []string
	collectUnknowns(val, "", &paths)
	if len(paths) == 0 {
		return nil
	}
	sort.Strings(paths)
	return paths
}

func collectUnknowns(val any, path string, out *[]string) {
	switch v := val.(type) {
	case string:
		if v == UnknownValue {
			if path == "" {
				*out = append(*out, "<root>")
			} else {
				*out = append(*out, path)
			}
		}
	case map[string]any:
		for k, mv := range v {
			child := k
			if path != "" {
				child = path + "." + k
			}
			collectUnknowns(mv, child, out)
		}
	case []any:
		for i, sv := range v {
			collectUnknowns(sv, fmt.Sprintf("%s[%d]", path, i), out)
		}
	}
}

// CtyToGo converts a cty.Value to a Go value for JSON serialization.
// Unknown leaves become the literal string "__cty_unknown__".
//
// Marks are stripped as the value is converted: a Go value carries no cty
// marks, and this is the boundary where a scope value (which may be
// sensitivity-marked, e.g. a reference to random_password.x.result) becomes a
// plain Go config value bound for a provider. Unmarking here is both required
// (AsString/ElementIterator panic on a marked value) and correct (marks must
// never cross to the provider wire). Sensitivity that must survive — e.g. an
// output that folds in a secret — is detected on the cty.Value via
// HasSensitiveMark before this conversion, not after.
func CtyToGo(val cty.Value) any {
	val, _ = val.Unmark()
	if val.IsNull() {
		return nil
	}
	if !val.IsKnown() {
		return UnknownValue
	}

	switch {
	case val.Type() == cty.String:
		return val.AsString()
	case val.Type() == cty.Number:
		bf := val.AsBigFloat()
		if bf.IsInt() {
			i64, _ := bf.Int64()
			return i64
		}
		f64, _ := bf.Float64()
		return f64
	case val.Type() == cty.Bool:
		return val.True()
	case val.Type().IsListType() || val.Type().IsTupleType() || val.Type().IsSetType():
		var result []any
		for it := val.ElementIterator(); it.Next(); {
			_, v := it.Element()
			result = append(result, CtyToGo(v))
		}
		return result
	case val.Type().IsMapType() || val.Type().IsObjectType():
		result := make(map[string]any)
		for it := val.ElementIterator(); it.Next(); {
			k, v := it.Element()
			result[k.AsString()] = CtyToGo(v)
		}
		return result
	default:
		return val.GoString()
	}
}

// GoToCty converts a Go value to a cty.Value.
func GoToCty(val any) cty.Value {
	if val == nil {
		return cty.NullVal(cty.DynamicPseudoType)
	}

	switch v := val.(type) {
	case string:
		if v == UnknownValue {
			return cty.UnknownVal(cty.DynamicPseudoType)
		}
		return cty.StringVal(v)
	case int:
		return cty.NumberIntVal(int64(v))
	case int64:
		return cty.NumberIntVal(v)
	case float64:
		return cty.NumberFloatVal(v)
	case bool:
		return cty.BoolVal(v)
	case []any:
		if len(v) == 0 {
			return cty.ListValEmpty(cty.DynamicPseudoType)
		}
		vals := make([]cty.Value, len(v))
		for i, elem := range v {
			vals[i] = GoToCty(elem)
		}
		return cty.TupleVal(vals)
	case map[string]any:
		if len(v) == 0 {
			return cty.EmptyObjectVal
		}
		vals := make(map[string]cty.Value, len(v))
		for k, elem := range v {
			vals[k] = GoToCty(elem)
		}
		return cty.ObjectVal(vals)
	default:
		return cty.StringVal(fmt.Sprintf("%v", v))
	}
}
