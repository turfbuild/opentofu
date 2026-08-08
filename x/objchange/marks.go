// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package objchange

import (
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/zclconf/go-cty/cty"
)

// MarkSensitive derives schema-driven sensitivity for val from the schema
// block (via configschema's ValueMarks) and returns val with the resulting
// path marks applied. This is the display/redaction path: the subject is nil
// so no deprecation marks are added.
func MarkSensitive(schema *configschema.Block, val cty.Value) cty.Value {
	return val.MarkWithPaths(schema.ValueMarks(val, nil, nil))
}

// SensitivePaths returns the Sensitive-only path marks already carried on val,
// in the form OpenTofu persists as ResourceInstanceObjectSrc.AttrSensitivePaths
// and re-applies via cty.Value.MarkWithPaths. It exists so callers can harvest
// the persistable subset of a value's marks without naming the internal
// marks.Sensitive symbol.
//
// This is deliberately NOT the schema-derived sensitivity (that is re-derived
// at display via MarkSensitive and never persisted). Only marks that already
// live on the value — e.g. a sensitive variable flowing into an attribute —
// are returned, and any non-Sensitive marks (e.g. Ephemeral, which must never
// reach state) are dropped. Returns nil when nothing sensitive is present.
func SensitivePaths(val cty.Value) []cty.PathValueMarks {
	_, pvms := val.UnmarkDeepWithPaths()
	var out []cty.PathValueMarks
	for _, pvm := range pvms {
		if _, ok := pvm.Marks[marks.Sensitive]; ok {
			out = append(out, cty.PathValueMarks{
				Path:  pvm.Path,
				Marks: cty.NewValueMarks(marks.Sensitive),
			})
		}
	}
	return out
}

// SensitiveValues projects the schema-derived sensitivity of val into the shape
// OpenTofu exposes as `sensitive_values` in `tofu show -json`: the same nesting
// as the value, but with `true` at every path that is sensitive and
// non-sensitive entries omitted. Returns nil when nothing is sensitive.
func SensitiveValues(schema *configschema.Block, val cty.Value) map[string]any {
	m, _ := sensitiveAsBool(MarkSensitive(schema, val)).(map[string]any)
	return m
}

// SensitiveValuesFromMarked projects an already-marked value into the
// `sensitive_values` shape (true at every sensitive path, non-sensitive entries
// omitted). Unlike SensitiveValues it does not re-derive from schema, so it
// reflects config-derived marks the caller has applied via MarkWithPaths in
// addition to schema-derived ones. Returns nil when nothing is sensitive.
func SensitiveValuesFromMarked(val cty.Value) map[string]any {
	m, _ := sensitiveAsBool(val).(map[string]any)
	return m
}

// sensitiveAsBool walks a mark-carrying value and returns a Go structure with
// `true` at sensitive nodes (redacting the whole subtree beneath), recursing
// into containers and omitting non-sensitive entries. Mirrors OpenTofu's
// jsonstate.SensitiveAsBool. Iteration is only ever reached on an unmarked
// node (a marked container short-circuits to true), so ElementIterator never
// panics on a marked value.
func sensitiveAsBool(val cty.Value) any {
	if val.HasMark(marks.Sensitive) {
		return true
	}
	if !val.IsKnown() || val.IsNull() {
		return nil
	}
	ty := val.Type()
	switch {
	case ty.IsPrimitiveType(), ty.Equals(cty.DynamicPseudoType):
		return nil
	case ty.IsObjectType(), ty.IsMapType():
		out := map[string]any{}
		for it := val.ElementIterator(); it.Next(); {
			k, ev := it.Element()
			if s := sensitiveAsBool(ev); s != nil {
				out[k.AsString()] = s
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case ty.IsListType(), ty.IsSetType(), ty.IsTupleType():
		var out []any
		found := false
		for it := val.ElementIterator(); it.Next(); {
			_, ev := it.Element()
			s := sensitiveAsBool(ev)
			out = append(out, s)
			if s != nil {
				found = true
			}
		}
		if !found {
			return nil
		}
		return out
	}
	return nil
}
