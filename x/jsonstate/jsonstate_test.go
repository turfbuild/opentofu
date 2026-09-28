// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package jsonstate_test

import (
	"encoding/json"
	"testing"

	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	xaddrs "github.com/opentofu/opentofu/x/addrs"
	xconfigs "github.com/opentofu/opentofu/x/configs"
	xjsonstate "github.com/opentofu/opentofu/x/jsonstate"
	xlang "github.com/opentofu/opentofu/x/lang"
	xstate "github.com/opentofu/opentofu/x/state"
	xstatefile "github.com/opentofu/opentofu/x/statefile"
	xtofu "github.com/opentofu/opentofu/x/tofu"
)

// TestMarshal renders a statefile assembled entirely from facade types — the
// property that makes a `tofu show -json` state export reachable from outside
// this module — and reads the document back as a consumer would.
func TestMarshal(t *testing.T) {
	provider := xaddrs.NewProvider("registry.opentofu.org", "hashicorp", "random")
	addr, err := xaddrs.ParseAbsResourceInstance(`random_pet.name["a"]`)
	if err != nil {
		t.Fatal(err)
	}

	st := xstate.NewState()
	st.EnsureModule(addr.Module).SetResourceInstanceCurrent(addr.Resource, &xstate.ResourceInstanceObjectSrc{
		AttrsJSON:     []byte(`{"id":"fond-fox","length":2}`),
		Status:        xstate.ObjectReady,
		SchemaVersion: 1,
	}, xaddrs.RootProviderConfig(provider), xaddrs.NoKey)

	schemas := xtofu.NewSchemas()
	xtofu.SetProvider(schemas, provider, xtofu.ProviderSchemas{
		Config: &xconfigs.Block{},
		Resources: map[string]xtofu.ResourceSchema{
			"random_pet": {
				Version: 1,
				Block: &xconfigs.Block{Attributes: map[string]*xconfigs.Attribute{
					"id":     {Type: cty.String, Computed: true},
					"length": {Type: cty.Number, Optional: true},
				}},
			},
		},
	})

	raw, err := xjsonstate.Marshal(xstatefile.New(st, "", 0), schemas)
	if err != nil {
		t.Fatal(err)
	}
	var doc xjsonstate.State
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.FormatVersion != xjsonstate.FormatVersion {
		t.Errorf("format_version = %q", doc.FormatVersion)
	}
	if doc.Values == nil || len(doc.Values.RootModule.Resources) != 1 {
		t.Fatalf("values = %s", raw)
	}
	res := doc.Values.RootModule.Resources[0]
	if res.Address != `random_pet.name["a"]` || res.SchemaVersion != 1 || res.ProviderName != "registry.opentofu.org/hashicorp/random" {
		t.Errorf("resource = %+v", res)
	}
	if string(res.AttributeValues["id"]) != `"fond-fox"` {
		t.Errorf("attribute id = %s", res.AttributeValues["id"])
	}

	// sensitive_values, from a marked value.
	marked := cty.ObjectVal(map[string]cty.Value{
		"id":     cty.StringVal("x"),
		"secret": xlang.MarkSensitive(cty.StringVal("s")),
	})
	projected := xjsonstate.SensitiveAsBool(marked)
	sv, err := ctyjson.Marshal(projected, projected.Type())
	if err != nil {
		t.Fatal(err)
	}
	if string(sv) != `{"secret":true}` {
		t.Errorf("sensitive_values = %s", sv)
	}
}
