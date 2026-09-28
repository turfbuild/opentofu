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
