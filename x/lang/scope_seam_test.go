// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package lang_test

import (
	"context"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	xlang "github.com/opentofu/opentofu/x/lang"
)

// The closed Scope and the EvalScope seam are one evaluator with two front
// doors: Scope converts to an EvalScope and every entry point that takes a
// *Scope goes through that conversion. This pins what the conversion has to
// carry, one case per kind of reference the closed scope can resolve, and
// checks the answer through both doors.
//
// The values, not just the agreement, are the point. Once the two paths share
// a conversion, "they agree" is nearly free — both would degrade together if
// the conversion dropped something. Asserting what each reference *resolves
// to* is what catches that, and it is the exact failure this package already
// had once: `caller` was carried on one surface and missing from the other.
func TestScopeResolvesEveryKindThroughTheSeam(t *testing.T) {
	obj := func(kv map[string]cty.Value) cty.Value { return cty.ObjectVal(kv) }

	build := func() *xlang.Scope {
		s := xlang.NewScope()
		s.SetVariable("v", cty.StringVal("var-value"))
		s.SetLocal("l", cty.NumberIntVal(3))
		s.SetResource("aws_thing.r", obj(map[string]cty.Value{"id": cty.StringVal("r-1")}))
		s.SetResource("aws_thing.keyed[0]", obj(map[string]cty.Value{"id": cty.StringVal("k-0")}))
		s.SetDataSource("aws_ami.a", obj(map[string]cty.Value{"id": cty.StringVal("ami-1")}))
		s.SetEphemeral("vault_secret.s", obj(map[string]cty.Value{"token": cty.StringVal("t")}))
		s.SetModuleOutput("m", obj(map[string]cty.Value{"out": cty.StringVal("m-out")}))
		s.Path = xlang.PathData{Module: "/mod", Root: "/root", Cwd: "/cwd"}
		s.Workspace = "staging"
		idx := 2
		s.Count = &idx
		s.Each = &xlang.EachData{Key: cty.StringVal("ek"), Value: cty.StringVal("ev")}
		s.Caller = obj(map[string]cty.Value{"id": cty.StringVal("caller-1")})
		return s
	}

	cases := []struct {
		expr string
		want cty.Value
	}{
		{"var.v", cty.StringVal("var-value")},
		{"local.l", cty.NumberIntVal(3)},
		{"aws_thing.r.id", cty.StringVal("r-1")},
		{"aws_thing.keyed[0].id", cty.StringVal("k-0")},
		{"data.aws_ami.a.id", cty.StringVal("ami-1")},
		{"ephemeral.vault_secret.s.token", cty.StringVal("t")},
		{"module.m.out", cty.StringVal("m-out")},
		{"path.module", cty.StringVal("/mod")},
		{"path.root", cty.StringVal("/root")},
		{"path.cwd", cty.StringVal("/cwd")},
		{"terraform.workspace", cty.StringVal("staging")},
		{"count.index", cty.NumberIntVal(2)},
		{"each.key", cty.StringVal("ek")},
		{"each.value", cty.StringVal("ev")},
		{"caller.id", cty.StringVal("caller-1")},
		{"upper(var.v)", cty.StringVal("VAR-VALUE")},
		// The closed scope is permissive by contract: an absent reference
		// resolves to unknown rather than raising, and the walkers depend on
		// that unknown propagating. The conversion must not make it strict.
		{"var.absent", cty.DynamicVal},
		{"aws_thing.absent.id", cty.DynamicVal},
		{"data.aws_ami.absent.id", cty.DynamicVal},
		{"module.absent.out", cty.DynamicVal},
	}

	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			parsed, diags := hclsyntax.ParseExpression([]byte(tc.expr), "test.hcl", hcl.InitialPos)
			if diags.HasErrors() {
				t.Fatalf("parsing %q: %s", tc.expr, diags.Error())
			}

			got, err := xlang.EvalExpression(parsed, build())
			if err != nil {
				t.Fatalf("through Scope: %v", err)
			}
			if !got.Value.RawEquals(tc.want) {
				t.Errorf("through Scope, %q = %#v, want %#v", tc.expr, got.Value, tc.want)
			}

			viaSeam, seamDiags := build().EvalScope().EvalExpr(context.Background(), parsed, cty.DynamicPseudoType)
			if seamDiags.HasErrors() {
				t.Fatalf("through EvalScope: %s", seamDiags.Err())
			}
			if !viaSeam.RawEquals(tc.want) {
				t.Errorf("through EvalScope, %q = %#v, want %#v", tc.expr, viaSeam, tc.want)
			}
		})
	}
}

// The conversion is a view, not a copy: a scope mutated after conversion --
// which is how the walkers drive count and for_each, rebinding the repetition
// values between instances -- must be visible through an EvalScope already
// taken from it.
func TestEvalScopeSeesLaterScopeMutations(t *testing.T) {
	s := xlang.NewScope()
	es := s.EvalScope()

	parsed, diags := hclsyntax.ParseExpression([]byte("count.index"), "test.hcl", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("parsing: %s", diags.Error())
	}

	idx := 7
	s.Count = &idx

	val, evalDiags := es.EvalExpr(context.Background(), parsed, cty.Number)
	if evalDiags.HasErrors() {
		t.Fatalf("eval: %s", evalDiags.Err())
	}
	if !val.RawEquals(cty.NumberIntVal(7)) {
		t.Errorf("count.index = %#v through a scope converted before Count was set", val)
	}
}
