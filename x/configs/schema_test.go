// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// TestStaticValidateTraversal pins the wrapper's two sides: a rule the schema
// has passes, and a rule it has not comes back as an error in the validate
// walk's words — the ones a host reports for a bad ignore_changes element.
func TestStaticValidateTraversal(t *testing.T) {
	block := &Block{
		Attributes: map[string]*Attribute{
			"ami": {Type: cty.String, Optional: true},
		},
		BlockTypes: map[string]*NestedBlock{
			"rule": {
				Nesting: NestingSet,
				Block: Block{Attributes: map[string]*Attribute{
					"name": {Type: cty.String, Optional: true},
				}},
			},
		},
	}
	// A rule as the decoder keeps it: relative, its first step an attribute
	// (configs.decodeResourceBlock, hcl.RelTraversalForExpr).
	parse := func(src string) hcl.Traversal {
		t.Helper()
		expr, diags := hclsyntax.ParseExpression([]byte(src), "", hcl.Pos{})
		if diags.HasErrors() {
			t.Fatal(diags)
		}
		traversal, diags := hcl.RelTraversalForExpr(expr)
		if diags.HasErrors() {
			t.Fatal(diags)
		}
		return traversal
	}

	if err := StaticValidateTraversal(block, parse("ami")); err != nil {
		t.Errorf("an attribute the schema has was refused: %v", err)
	}
	err := StaticValidateTraversal(block, parse("nope"))
	if err == nil || !strings.Contains(err.Error(), "Unsupported attribute") || !strings.Contains(err.Error(), `named "nope"`) {
		t.Errorf("an unknown attribute must be refused in the validate walk's words; got %v", err)
	}
	err = StaticValidateTraversal(block, parse("rule[0]"))
	if err == nil || !strings.Contains(err.Error(), "Cannot index a set value") {
		t.Errorf("an indexed set block must be refused in the validate walk's words; got %v", err)
	}
}
