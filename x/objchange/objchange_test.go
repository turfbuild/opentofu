// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package objchange

import (
	"errors"
	"strings"
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

// TestAssertObjectCompatible pins the plan-against-plan check as a host calls
// it: an unknown planned value may become known, a known one may not change,
// and the error names the path that moved.
func TestAssertObjectCompatible(t *testing.T) {
	schema := &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"name":   {Type: cty.String, Required: true},
			"id":     {Type: cty.String, Computed: true},
			"secret": {Type: cty.String, Optional: true, Sensitive: true},
		},
	}
	obj := func(name, id, secret cty.Value) cty.Value {
		return cty.ObjectVal(map[string]cty.Value{"name": name, "id": id, "secret": secret})
	}
	planned := obj(cty.StringVal("web"), cty.UnknownVal(cty.String), cty.StringVal("s1"))

	if errs := AssertObjectCompatible(schema, planned, obj(cty.StringVal("web"), cty.StringVal("i-1"), cty.StringVal("s1"))); len(errs) != 0 {
		t.Errorf("an unknown becoming known reported %v; want compatible", errs)
	}

	errs := AssertObjectCompatible(schema, planned, obj(cty.StringVal("db"), cty.StringVal("i-1"), cty.StringVal("s1")))
	if len(errs) != 1 {
		t.Fatalf("a changed known value reported %v; want one error", errs)
	}
	var pathErr cty.PathError
	if !errors.As(errs[0], &pathErr) || !pathErr.Path.Equals(cty.GetAttrPath("name")) {
		t.Errorf("the error %v does not name the path name", errs[0])
	}

	errs = AssertObjectCompatible(schema, planned, obj(cty.StringVal("web"), cty.StringVal("i-1"), cty.StringVal("s2")))
	if len(errs) != 1 || strings.Contains(errs[0].Error(), "s1") || strings.Contains(errs[0].Error(), "s2") {
		t.Errorf("a changed sensitive value reported %v; want one error that names neither value", errs)
	}
}
