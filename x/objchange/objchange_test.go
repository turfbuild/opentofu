// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package objchange

import (
	"testing"

	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/zclconf/go-cty/cty"
)

// TestPlannedUnknownObject pins the provider-less plan: configured values are
// kept, and a computed attribute the configuration leaves unset is unknown —
// not null, which is what ProposedNew over a null prior would say, and which
// would read as "the provider decided there is no value".
func TestPlannedUnknownObject(t *testing.T) {
	schema := &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"name": {Type: cty.String, Required: true},
			"id":   {Type: cty.String, Computed: true},
			"tags": {Type: cty.Map(cty.String), Optional: true, Computed: true},
		},
	}
	config := cty.ObjectVal(map[string]cty.Value{
		"name": cty.StringVal("web"),
		"id":   cty.NullVal(cty.String),
		"tags": cty.NullVal(cty.Map(cty.String)),
	})

	got := PlannedUnknownObject(schema, config)
	if !got.GetAttr("name").RawEquals(cty.StringVal("web")) {
		t.Errorf("name = %#v, want the configured value", got.GetAttr("name"))
	}
	for _, attr := range []string{"id", "tags"} {
		if got.GetAttr(attr).IsKnown() {
			t.Errorf("%s = %#v, want unknown", attr, got.GetAttr(attr))
		}
	}

	if proposed := ProposedNew(schema, cty.NullVal(schema.ImpliedType()), config); !proposed.GetAttr("id").IsNull() {
		t.Errorf("ProposedNew over a null prior planned id as %#v; the contrast this test draws is gone", proposed.GetAttr("id"))
	}
}

// TestNormalizeObjectFromLegacySDK pins the refresh repair: a null list block
// becomes an empty list, a null single block stays null, attributes pass
// through untouched, and a null object stays null.
func TestNormalizeObjectFromLegacySDK(t *testing.T) {
	check := &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"interval": {Type: cty.String, Optional: true},
		},
	}
	schema := &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"name": {Type: cty.String, Required: true},
			"tags": {Type: cty.Map(cty.String), Optional: true},
		},
		BlockTypes: map[string]*configschema.NestedBlock{
			"healthcheck": {Nesting: configschema.NestingList, Block: *check},
			"single":      {Nesting: configschema.NestingSingle, Block: *check},
		},
	}
	ty := schema.ImpliedType()
	val := cty.ObjectVal(map[string]cty.Value{
		"name":        cty.StringVal("web"),
		"tags":        cty.NullVal(cty.Map(cty.String)),
		"healthcheck": cty.NullVal(ty.AttributeType("healthcheck")),
		"single":      cty.NullVal(ty.AttributeType("single")),
	})

	got := NormalizeObjectFromLegacySDK(schema, val)
	if hc := got.GetAttr("healthcheck"); hc.IsNull() || hc.LengthInt() != 0 {
		t.Errorf("healthcheck = %#v, want an empty list", hc)
	}
	if !got.GetAttr("single").IsNull() {
		t.Errorf("single = %#v, want null: only list, set and group blocks are filled in", got.GetAttr("single"))
	}
	if !got.GetAttr("name").RawEquals(cty.StringVal("web")) || !got.GetAttr("tags").IsNull() {
		t.Errorf("attributes changed: name %#v, tags %#v", got.GetAttr("name"), got.GetAttr("tags"))
	}
	if null := NormalizeObjectFromLegacySDK(schema, cty.NullVal(ty)); !null.IsNull() {
		t.Errorf("a null object normalized to %#v, want null", null)
	}
}
