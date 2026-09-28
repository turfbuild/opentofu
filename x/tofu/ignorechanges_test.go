// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/configs/configschema"
)

// TestProcessIgnoreChangesRevertsThePrior pins the wrapper end to end: a rule
// as the configuration decoder keeps it, converted, applied — the ignored
// attribute takes the prior's value and its marks, the other the
// configuration's; and "all" hands back the prior with its computed-only
// attribute nulled.
func TestProcessIgnoreChangesRevertsThePrior(t *testing.T) {
	// The rule as the decoder keeps it: relative (hcl.RelTraversalForExpr).
	expr, diags := hclsyntax.ParseExpression([]byte("ami"), "", hcl.Pos{})
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	traversal, diags := hcl.RelTraversalForExpr(expr)
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	rules := []cty.Path{TraversalToPath(traversal)}
	schema := &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"id":   {Type: cty.String, Computed: true},
			"ami":  {Type: cty.String, Optional: true},
			"name": {Type: cty.String, Optional: true},
		},
	}
	prior := cty.ObjectVal(map[string]cty.Value{
		"id":   cty.StringVal("i-1"),
		"ami":  cty.StringVal("ami-1").Mark("kept"),
		"name": cty.StringVal("old"),
	})
	config := cty.ObjectVal(map[string]cty.Value{
		"id":   cty.NullVal(cty.String),
		"ami":  cty.StringVal("ami-2"),
		"name": cty.StringVal("new"),
	})

	got, err := ProcessIgnoreChanges(prior, config, schema, rules, false)
	if err != nil {
		t.Fatal(err)
	}
	want := cty.ObjectVal(map[string]cty.Value{
		"id":   cty.NullVal(cty.String),
		"ami":  cty.StringVal("ami-1").Mark("kept"),
		"name": cty.StringVal("new"),
	})
	if !got.RawEquals(want) {
		t.Errorf("one rule: got %#v\nwant %#v", got, want)
	}

	all, err := ProcessIgnoreChanges(prior, config, schema, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	wantAll := cty.ObjectVal(map[string]cty.Value{
		"id":   cty.NullVal(cty.String),
		"ami":  cty.StringVal("ami-1").Mark("kept"),
		"name": cty.StringVal("old"),
	})
	if !all.RawEquals(wantAll) {
		t.Errorf("all: got %#v\nwant %#v", all, wantAll)
	}

	if created, _ := ProcessIgnoreChanges(cty.NullVal(prior.Type()), config, schema, rules, false); !created.RawEquals(config) {
		t.Errorf("a null prior must ignore nothing; got %#v", created)
	}
}
