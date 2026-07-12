// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package lang

import (
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// assertExprResult parses an HCL expression, evaluates it against the
// given scope via the OTF adapter, and asserts the result equals want.
// This is the end-to-end test contract — what the OTF evaluator surfaces
// for ${...} references — rather than implementation details of any
// particular EvalContext layout.
func assertExprResult(t *testing.T, s *Scope, exprSrc string, want cty.Value) {
	t.Helper()
	expr, diags := hclsyntax.ParseExpression([]byte(exprSrc), "test.hcl", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatalf("parse %q: %s", exprSrc, diags.Error())
	}
	res, err := EvalExpression(expr, s)
	if err != nil {
		t.Fatalf("eval %q: %v", exprSrc, err)
	}
	if !res.Value.RawEquals(want) {
		t.Errorf("eval %q = %#v; want %#v", exprSrc, res.Value, want)
	}
}

func TestScopeResolvesModuleOutputs(t *testing.T) {
	s := NewScope()

	s.SetModuleOutput("interfaces", cty.ObjectVal(map[string]cty.Value{
		"normalized_assignments": cty.MapVal(map[string]cty.Value{
			"reader": cty.ObjectVal(map[string]cty.Value{
				"principal_id": cty.StringVal("alice"),
			}),
		}),
		"primary_id": cty.StringVal("iface-1"),
	}))

	assertExprResult(t, s, "module.interfaces.primary_id", cty.StringVal("iface-1"))
	assertExprResult(t, s,
		`module.interfaces.normalized_assignments["reader"].principal_id`,
		cty.StringVal("alice"))
}

func TestScopeResolvesCountKeyedModuleOutputs(t *testing.T) {
	s := NewScope()

	// Two instances of `module.subnet` with count = 2.
	s.SetModuleOutput("subnet", cty.TupleVal([]cty.Value{
		cty.ObjectVal(map[string]cty.Value{
			"id":   cty.StringVal("sn-0"),
			"cidr": cty.StringVal("10.0.1.0/24"),
		}),
		cty.ObjectVal(map[string]cty.Value{
			"id":   cty.StringVal("sn-1"),
			"cidr": cty.StringVal("10.0.2.0/24"),
		}),
	}))

	assertExprResult(t, s, "module.subnet[0].id", cty.StringVal("sn-0"))
	assertExprResult(t, s, "module.subnet[1].cidr", cty.StringVal("10.0.2.0/24"))
	assertExprResult(t, s, "module.subnet[*].id", cty.TupleVal([]cty.Value{
		cty.StringVal("sn-0"),
		cty.StringVal("sn-1"),
	}))
}

func TestScopeResolvesForEachKeyedModuleOutputs(t *testing.T) {
	s := NewScope()

	s.SetModuleOutput("region", cty.MapVal(map[string]cty.Value{
		"east": cty.ObjectVal(map[string]cty.Value{
			"name": cty.StringVal("us-east-1"),
			"id":   cty.StringVal("r-east"),
		}),
		"west": cty.ObjectVal(map[string]cty.Value{
			"name": cty.StringVal("us-west-2"),
			"id":   cty.StringVal("r-west"),
		}),
	}))

	assertExprResult(t, s, `module.region["east"].name`, cty.StringVal("us-east-1"))
	assertExprResult(t, s, `module.region["west"].id`, cty.StringVal("r-west"))
}

func TestScopeMixesKeyedAndUnkeyedModules(t *testing.T) {
	s := NewScope()
	s.SetModuleOutput("naming", cty.ObjectVal(map[string]cty.Value{
		"prefix": cty.StringVal("core"),
	}))
	s.SetModuleOutput("subnet", cty.TupleVal([]cty.Value{
		cty.ObjectVal(map[string]cty.Value{"id": cty.StringVal("sn-0")}),
	}))
	s.SetModuleOutput("region", cty.MapVal(map[string]cty.Value{
		"east": cty.ObjectVal(map[string]cty.Value{"id": cty.StringVal("r-east")}),
	}))

	assertExprResult(t, s, "module.naming.prefix", cty.StringVal("core"))
	assertExprResult(t, s, "module.subnet[0].id", cty.StringVal("sn-0"))
	assertExprResult(t, s, `module.region["east"].id`, cty.StringVal("r-east"))
}

// TestScopeModuleResolvesDeclaredOutputsOnly asserts the Terraform-conforming
// contract: a module call is addressable only through its declared outputs
// (SetModuleOutput). Resource state stored at module-prefixed addresses stays
// inert — only local-form (type.name) resource keys resolve, and a reference
// into a module's internals fails validation at output granularity.
func TestScopeModuleResolvesDeclaredOutputsOnly(t *testing.T) {
	parse := func(src string) hcl.Expression {
		expr, diags := hclsyntax.ParseExpression([]byte(src), "test.hcl", hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			t.Fatalf("parse %q: %s", src, diags.Error())
		}
		return expr
	}

	s := NewScope()
	s.SetResource("random_pet.flat", cty.ObjectVal(map[string]cty.Value{
		"id": cty.StringVal("flat-id"),
	}))
	// Full state addresses may be seeded into the scope; they must not make
	// the module namespace resolvable.
	s.SetResource("module.first.random_pet.anchor", cty.ObjectVal(map[string]cty.Value{
		"id": cty.StringVal("anchor-id"),
	}))

	assertExprResult(t, s, "random_pet.flat.id", cty.StringVal("flat-id"))

	err := ValidateExpression(parse("module.first.random_pet.anchor.id"), s)
	if err == nil {
		t.Fatal("module-internal resource reference validated; want rejection")
	}
	if !strings.Contains(err.Error(), "module.first") {
		t.Fatalf("error does not name the module reference: %v", err)
	}

	// With outputs registered, the declared output resolves; the internal
	// resource still does not.
	s.SetModuleOutput("first", cty.ObjectVal(map[string]cty.Value{
		"pet_id": cty.StringVal("anchor-id"),
	}))
	assertExprResult(t, s, "module.first.pet_id", cty.StringVal("anchor-id"))
	err = ValidateExpression(parse("module.first.random_pet.anchor.id"), s)
	if err == nil {
		t.Fatal("module-internal resource reference validated despite registered outputs")
	}
	if !strings.Contains(err.Error(), "declared outputs") {
		t.Fatalf("error lacks the declared-outputs guidance: %v", err)
	}
}
