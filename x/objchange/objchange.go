// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

// Package objchange is a stable boundary over OpenTofu's internal
// plans/objchange package (ProposedNew, PlannedUnknownObject) and the
// schema-driven sensitivity marks built on configschema.
package objchange

import (
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/plans/objchange"
	"github.com/zclconf/go-cty/cty"
)

// ProposedNew computes the proposed new state by merging prior state with
// configuration, using the schema to determine which attributes should come
// from which source.
//
// This wraps OpenTofu's objchange.ProposedNew function.
//
// For each attribute:
//   - Computed attributes with null config: use prior state value
//   - Computed attributes with non-null config: use config value (explicit override)
//   - OptionalComputed attributes with null config: use prior state value
//   - OptionalComputed attributes with non-null config: use config value
//   - Optional/Required attributes: always use config value
//
// For nested blocks, the merge is recursive with correlation based on
// nesting mode (list by index, map by key, set by heuristic).
func ProposedNew(schema *configschema.Block, prior, config cty.Value) cty.Value {
	return objchange.ProposedNew(schema, prior, config)
}

// PlannedUnknownObject is the planned value of an object whose planning is
// handled without a provider — or deferred before one is asked: the
// configuration's values where it sets them, and unknown for every computed
// attribute it leaves unset. It is what OpenTofu plans for a data source it
// cannot read yet, and for a deferred ephemeral resource.
//
// This wraps OpenTofu's objchange.PlannedUnknownObject.
func PlannedUnknownObject(schema *configschema.Block, config cty.Value) cty.Value {
	return objchange.PlannedUnknownObject(schema, config)
}
