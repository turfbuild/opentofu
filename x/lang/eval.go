// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package lang

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/zclconf/go-cty/cty"
)

// EvalResult represents the result of evaluating an expression.
type EvalResult struct {
	// Value is the evaluated value
	Value cty.Value

	// IsKnown indicates whether the value is fully known (no unknowns)
	IsKnown bool
}

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

// EvalExpression evaluates an HCL expression via OpenTofu's lang.Scope.
// Missing references resolve to cty.DynamicVal so unknowns propagate
// through cty semantics. To fail fast on missing references, call
// ValidateExpression first.
func EvalExpression(expr hcl.Expression, scope *Scope) (*EvalResult, error) {
	ctx := context.Background()
	val, diags := scope.EvalScope().EvalExpr(ctx, expr, cty.DynamicPseudoType)
	if diags.HasErrors() {
		return nil, fmt.Errorf("evaluation error: %s", diags.Err())
	}
	return &EvalResult{
		Value:   val,
		IsKnown: val.IsWhollyKnown(),
	}, nil
}

// EvalExpressionString parses and evaluates an HCL template string. A string
// with no interpolation/directive sequence passes through as cty.String. A
// single-interpolation template ("${var.foo}") preserves the interpolation's
// native type (HCL's TemplateWrapExpr), while a mixed template ("${a}-${b}")
// evaluates to the concatenated string.
func EvalExpressionString(s string, scope *Scope) (*EvalResult, error) {
	if !ContainsInterpolation(s) {
		return &EvalResult{Value: cty.StringVal(s), IsKnown: true}, nil
	}
	expr, diags := hclsyntax.ParseTemplate([]byte(s), "", hcl.InitialPos)
	if diags.HasErrors() {
		return nil, fmt.Errorf("parse error: %s", diags.Error())
	}
	return EvalExpression(expr, scope)
}

// EvalConfig evaluates a configuration map, recursing into nested maps
// and slices. Missing references resolve to cty.DynamicVal and propagate
// as __cty_unknown__ markers via CtyToGo. To fail fast on missing refs,
// call ValidateConfig before this.
func EvalConfig(config map[string]any, scope *Scope) (map[string]any, error) {
	result := make(map[string]any, len(config))
	for key, val := range config {
		ev, err := evalValue(val, scope)
		if err != nil {
			return nil, fmt.Errorf("error evaluating %s: %w", key, err)
		}
		result[key] = ev
	}
	return result, nil
}

func evalValue(val any, scope *Scope) (any, error) {
	switch v := val.(type) {
	case string:
		if ContainsInterpolation(v) {
			result, err := EvalExpressionString(v, scope)
			if err != nil {
				return nil, err
			}
			return CtyToGo(result.Value), nil
		}
		return v, nil
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, mv := range v {
			ev, err := evalValue(mv, scope)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", k, err)
			}
			out[k] = ev
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, sv := range v {
			ev, err := evalValue(sv, scope)
			if err != nil {
				return nil, fmt.Errorf("[%d]: %w", i, err)
			}
			out[i] = ev
		}
		return out, nil
	default:
		return v, nil
	}
}

// EvalConfigSensitivePaths walks config (typically the walker's
// already-evaluated map, whose remaining ${...} leaves get re-resolved against
// scope) and returns, for each top-level attribute containing a Sensitive value
// anywhere beneath it, a path marking that whole attribute. It is the map-eval
// counterpart to configs.DecodeBodySensitivePaths, used for the second-stage
// re-eval (module_plan) that EvalConfig performs — EvalConfig drops marks at the
// CtyToGo boundary, so config-flow sensitivity must be recovered separately.
//
// Unlike the schema-aware DecodeBodySensitivePaths, this sees only the untyped
// Go map, so it cannot tell a cty map attribute (IndexStep keys) from an object
// attribute (GetAttrStep keys). Emitting a precise nested path would risk the
// wrong step kind → MarkWithPaths silently missing it → a leak. It therefore
// marks at top-level-attribute granularity: exact for a top-level scalar
// (the common case), and a safe over-redaction for a sensitive value nested
// under a map/list/object attribute. The precise walk-time paths still come
// from DecodeBodySensitivePaths (unioned by the caller); this only adds coarse
// marks for references resolved for the first time during the re-eval.
func EvalConfigSensitivePaths(config map[string]any, scope *Scope) []cty.PathValueMarks {
	var out []cty.PathValueMarks
	for key, val := range config {
		if valueContainsSensitive(val, scope) {
			out = append(out, cty.PathValueMarks{
				Path:  cty.GetAttrPath(key),
				Marks: cty.NewValueMarks(marks.Sensitive),
			})
		}
	}
	return out
}

// valueContainsSensitive reports whether re-resolving val against scope surfaces
// a Sensitive mark anywhere. An expression that fails to evaluate carries no
// mark, so it is treated as not sensitive rather than fatal.
func valueContainsSensitive(val any, scope *Scope) bool {
	switch v := val.(type) {
	case string:
		if !ContainsInterpolation(v) {
			return false
		}
		res, err := EvalExpressionString(v, scope)
		if err != nil {
			return false
		}
		_, pvms := res.Value.UnmarkDeepWithPaths()
		for _, pvm := range pvms {
			if _, ok := pvm.Marks[marks.Sensitive]; ok {
				return true
			}
		}
	case map[string]any:
		for _, mv := range v {
			if valueContainsSensitive(mv, scope) {
				return true
			}
		}
	case []any:
		for _, sv := range v {
			if valueContainsSensitive(sv, scope) {
				return true
			}
		}
	}
	return false
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

// FindUnresolvedUnknowns returns the dotted paths where evaluated has an
// UnknownValue that original did not. Paths where original was already
// UnknownValue at plan time (e.g. unknowns baked in by module composition
// before the token was minted) are excluded — re-evaluation has no
// expression to re-resolve there, so reporting them as actionable would be
// misleading. Paths are sorted.
func FindUnresolvedUnknowns(original, evaluated any) []string {
	all := FindUnknowns(evaluated)
	if len(all) == 0 {
		return nil
	}
	preexisting := make(map[string]struct{})
	for _, p := range FindUnknowns(original) {
		preexisting[p] = struct{}{}
	}
	out := all[:0:0]
	for _, p := range all {
		if _, ok := preexisting[p]; ok {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
