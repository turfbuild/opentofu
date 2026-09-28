// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

// Package objchange is a stable boundary over OpenTofu's internal
// plans/objchange package (ProposedNew, PlannedUnknownObject,
// AssertObjectCompatible) and the schema-driven sensitivity marks built on
// configschema.
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

// NormalizeObjectFromLegacySDK repairs an object a legacy-SDK provider
// returned so that no list or set block in it is null or unknown: a null one
// becomes an empty collection and an unknown one a collection of one unknown
// stub (a null group block becomes its empty value; a single block is left as
// it is). That is what the protocol promises and the legacy SDK's type-system
// shim does not always deliver. The value must conform to the schema's
// implied type.
//
// OpenTofu applies it to every refreshed object, whatever SDK the provider
// was built with (node_resource_abstract_instance.go, refresh). Without it a
// null block read back from a legacy-SDK provider meets the empty block the
// configuration decodes to, and the provider — whose optional-computed blocks
// plan unknown from a null prior — forces a replace on a plan that has
// nothing to change.
//
// This wraps OpenTofu's objchange.NormalizeObjectFromLegacySDK.
func NormalizeObjectFromLegacySDK(schema *configschema.Block, val cty.Value) cty.Value {
	return objchange.NormalizeObjectFromLegacySDK(val, schema)
}

// AssertObjectCompatible reports how actual fails to be a valid completion of
// the possibly partly unknown planned value: a known planned leaf must stay
// equal, an unknown one may become any value its refinements allow, and a
// null or present object, a block count or a set's length must not change in
// the ways the plan ruled out. No errors means compatible. Both values must
// conform to the schema's implied type. A sensitive attribute, by schema or
// by a mark on either value, is reported without its values.
//
// OpenTofu runs it twice for every managed resource it applies: the
// apply-time re-plan against the approved plan ("Provider produced
// inconsistent final plan", node_resource_apply_instance.go,
// checkPlannedChange), and the applied object against the re-planned one
// ("Provider produced inconsistent result after apply",
// node_resource_abstract_instance.go, apply), the second tolerated for a
// provider on the legacy type system.
//
// The errors are usually cty.PathError values naming the path that differs.
// Their messages carry the values for a provider developer to read; a caller
// that must not disclose them renders the path alone.
//
// This wraps OpenTofu's objchange.AssertObjectCompatible.
func AssertObjectCompatible(schema *configschema.Block, planned, actual cty.Value) []error {
	return objchange.AssertObjectCompatible(schema, planned, actual)
}
