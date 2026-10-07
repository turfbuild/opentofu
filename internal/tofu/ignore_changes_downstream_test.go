// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/configs/configschema"
)

// TestProcessIgnoreChanges pins the exported entry point against the
// method's branches: a null prior, no rules, the individual revert, and "all"
// with and without a schema. The individual rules themselves are pinned by
// TestProcessIgnoreChangesIndividual.
func TestProcessIgnoreChanges(t *testing.T) {
	schema := &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"id":   {Type: cty.String, Computed: true},
			"ami":  {Type: cty.String, Optional: true},
			"name": {Type: cty.String, Optional: true, Computed: true},
		},
	}
	prior := cty.ObjectVal(map[string]cty.Value{
		"id":   cty.StringVal("i-1"),
		"ami":  cty.StringVal("ami-1"),
		"name": cty.StringVal("old"),
	})
	config := cty.ObjectVal(map[string]cty.Value{
		"id":   cty.NullVal(cty.String),
		"ami":  cty.StringVal("ami-2"),
		"name": cty.StringVal("new"),
	})
	ami := []cty.Path{cty.GetAttrPath("ami")}

	tests := map[string]struct {
		prior, config cty.Value
		schema        *configschema.Block
		ignore        []cty.Path
		all           bool
		want          cty.Value
	}{
		"null prior ignores nothing": {cty.NullVal(prior.Type()), config, schema, ami, false, config},
		"no rules":                   {prior, config, schema, nil, false, config},
		"one path reverts": {prior, config, schema, ami, false, cty.ObjectVal(map[string]cty.Value{
			"id":   cty.NullVal(cty.String),
			"ami":  cty.StringVal("ami-1"),
			"name": cty.StringVal("new"),
		})},
		"all with a schema nulls computed-only": {prior, config, schema, nil, true, cty.ObjectVal(map[string]cty.Value{
			"id":   cty.NullVal(cty.String),
			"ami":  cty.StringVal("ami-1"),
			"name": cty.StringVal("old"),
		})},
		"all without a schema is the prior": {prior, config, nil, nil, true, prior},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, diags := ProcessIgnoreChanges(tc.prior, tc.config, tc.schema, tc.ignore, tc.all)
			if diags.HasErrors() {
				t.Fatal(diags.Err())
			}
			if !got.RawEquals(tc.want) {
				t.Errorf("got %#v\nwant %#v", got, tc.want)
			}
		})
	}
}

func TestTraversalToPath(t *testing.T) {
	traversal, diags := hclsyntax.ParseTraversalAbs([]byte(`tags["pinned"].x[0]`), "", hcl.Pos{})
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	got := TraversalToPath(traversal)
	want := cty.GetAttrPath("tags").Index(cty.StringVal("pinned")).GetAttr("x").Index(cty.NumberIntVal(0))
	if !got.Equals(want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}
