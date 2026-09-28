// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package lang

import (
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/zclconf/go-cty/cty"
)

// HasSensitiveMark reports whether val carries the Sensitive mark at the top
// level. This matches OpenTofu's own meta-argument checks (which use
// cty.Value.HasMark): a set whose elements are sensitive becomes sensitive at
// the set level, so a top-level check is sufficient to catch a sensitive
// for_each. Used to reject sensitive count/for_each/enabled meta-arguments,
// where the value would otherwise leak via an instance key or instance presence.
func HasSensitiveMark(val cty.Value) bool {
	return marks.Has(val, marks.Sensitive)
}

// HasEphemeralMark reports whether val carries the Ephemeral mark at the top
// level. Ephemeral values must not drive cardinality for the same reason as
// sensitive ones (they would be exposed through instance keys / presence).
func HasEphemeralMark(val cty.Value) bool {
	return marks.Has(val, marks.Ephemeral)
}

// MarkSensitive returns val with the Sensitive mark applied at the top level —
// the constructive counterpart to HasSensitiveMark. It uses the same opaque
// mark value OpenTofu uses, so a value marked here is recognized as sensitive
// everywhere marks are inspected (HasSensitiveMark, UnmarkDeep, the redacting
// converter). The mark value is only reachable from inside this package, so this is
// the sanctioned way for root-module code to mark a value sensitive.
func MarkSensitive(val cty.Value) cty.Value {
	return val.Mark(marks.Sensitive)
}

// ContainsSensitiveMark reports whether val carries the Sensitive mark anywhere
// within it, not merely at the top level. Where HasSensitiveMark answers the
// cardinality question (a marked collection makes every instance key a leak),
// this answers the value question: an object whose one attribute is sensitive is
// as unusable as a wholly sensitive one for something that must be written down
// in the clear — an import identity, say. Matches OpenTofu's marks.Contains.
func ContainsSensitiveMark(val cty.Value) bool {
	return marks.Contains(val, marks.Sensitive)
}

// ContainsEphemeralMark is the same deep test for the Ephemeral mark.
func ContainsEphemeralMark(val cty.Value) bool {
	return marks.Contains(val, marks.Ephemeral)
}

// MarkEphemeral returns val with the Ephemeral mark applied at the top level —
// the constructive twin of MarkSensitive. OpenTofu applies this mark when an
// `ephemeral = true` input variable is read (evaluate.go's GetInputVariable);
// a root module driving the walk itself needs the same constructor so that
// ephemeral values it introduces are recognized by every mark inspection
// (ContainsEphemeralMark, UnmarkDeep, the non-ephemeral-context gates).
func MarkEphemeral(val cty.Value) cty.Value {
	return val.Mark(marks.Ephemeral)
}
