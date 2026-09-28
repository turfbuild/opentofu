// Copyright (c) The Turf Authors
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
