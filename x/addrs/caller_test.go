// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/opentofu/opentofu/internal/addrs"
)

// TestCaller pins that a parsed "caller" reference compares equal to the
// facade's Caller, so a consumer can recognize the subject by identity rather
// than by its rendered form.
func TestCaller(t *testing.T) {
	trav, diags := hclsyntax.ParseTraversalAbs([]byte("caller.id"), "", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	ref, refDiags := addrs.ParseRef(trav)
	if refDiags.HasErrors() {
		t.Fatal(refDiags.Err())
	}
	if ref.Subject != Caller {
		t.Fatalf("subject %#v is not Caller", ref.Subject)
	}
	var subject Referenceable = Caller
	if subject.String() != "caller" {
		t.Errorf("Caller renders as %q", subject.String())
	}
}

// TestSelf is TestCaller for "self": a postcondition's reference to the
// instance it checks is recognized by identity.
func TestSelf(t *testing.T) {
	trav, diags := hclsyntax.ParseTraversalAbs([]byte("self.id"), "", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	ref, refDiags := addrs.ParseRef(trav)
	if refDiags.HasErrors() {
		t.Fatal(refDiags.Err())
	}
	if ref.Subject != Self {
		t.Fatalf("subject %#v is not Self", ref.Subject)
	}
	var subject Referenceable = Self
	if subject.String() != "self" {
		t.Errorf("Self renders as %q", subject.String())
	}
}

// TestProviderFunction pins that a provider-defined function call parses to
// the facade's ProviderFunction, alias form included, so a consumer resolving
// one can name the subject it is handed.
func TestProviderFunction(t *testing.T) {
	for src, want := range map[string]ProviderFunction{
		"provider::time::rfc3339_parse":      {ProviderName: "time", Function: "rfc3339_parse"},
		"provider::aws::west::arn_parse":     {ProviderName: "aws", ProviderAlias: "west", Function: "arn_parse"},
		"provider::terraform::encode_tfvars": {ProviderName: "terraform", Function: "encode_tfvars"},
	} {
		expr, diags := hclsyntax.ParseExpression([]byte(src+"(1)"), "", hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			t.Fatal(diags)
		}
		call := expr.(*hclsyntax.FunctionCallExpr)
		ref, refDiags := addrs.ParseRef(hcl.Traversal{hcl.TraverseRoot{Name: call.Name, SrcRange: call.NameRange}})
		if refDiags.HasErrors() {
			t.Fatal(refDiags.Err())
		}
		got, ok := ref.Subject.(ProviderFunction)
		if !ok {
			t.Fatalf("%s: subject %#v is not a ProviderFunction", src, ref.Subject)
		}
		if got.ProviderName != want.ProviderName || got.ProviderAlias != want.ProviderAlias || got.Function != want.Function {
			t.Errorf("%s: parsed %#v", src, got)
		}
		if got.String() != src {
			t.Errorf("%s renders as %q", src, got.String())
		}
	}
}
