// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package lang_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	xaddrs "github.com/opentofu/opentofu/x/addrs"
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

// strictData is a Data that diagnoses an unresolvable reference instead of
// answering with an unknown. Real consumers do this: a walk that knows the
// value should already be there reports its absence as a defect in its own
// ordering, not as something to wait for.
type strictData struct {
	xlang.Data
}

func (strictData) StaticValidateReferences(context.Context, []*xaddrs.Reference, xaddrs.Referenceable, xaddrs.Referenceable) xlang.Diagnostics {
	return nil
}

func (strictData) GetResource(_ context.Context, addr xaddrs.Resource, _ xlang.SourceRange) (cty.Value, xlang.Diagnostics) {
	var diags xlang.Diagnostics
	return cty.DynamicVal, diags.Append(fmt.Errorf(
		"no value is available for %s: the walk read it before it was planned, a defect in the walk's ordering", addr))
}

// A repetition argument that cannot be evaluated is an error, not a deferral.
//
// The distinction only exists because the scope decides what an unresolvable
// reference looks like: a permissive backend answers with an unknown, which
// legitimately defers, while a strict one diagnoses. Deferring on a diagnostic
// would take a caller's ordering bug -- whose message says exactly that -- and
// turn it into an object that quietly defers every round and never converges.
func TestRepetitionRaisesAStrictScopesDiagnostic(t *testing.T) {
	scope := &xlang.EvalScope{Data: strictData{}}

	t.Run("count", func(t *testing.T) {
		expr, diags := hclsyntax.ParseExpression([]byte("length(aws_thing.r.names)"), "test.hcl", hcl.InitialPos)
		if diags.HasErrors() {
			t.Fatalf("parsing: %s", diags.Error())
		}
		_, deferred, err := xlang.EvaluateCount(expr, scope)
		if deferred {
			t.Fatal("deferred on a scope that diagnosed the reference rather than deferring it")
		}
		if err == nil {
			t.Fatal("no error from a count whose reference the scope refused")
		}
	})

	t.Run("for_each", func(t *testing.T) {
		expr, diags := hclsyntax.ParseExpression([]byte("aws_thing.r.names"), "test.hcl", hcl.InitialPos)
		if diags.HasErrors() {
			t.Fatalf("parsing: %s", diags.Error())
		}
		_, deferred, err := xlang.EvaluateForEach(expr, scope, false, true)
		if deferred {
			t.Fatal("deferred on a scope that diagnosed the reference rather than deferring it")
		}
		if err == nil {
			t.Fatal("no error from a for_each whose reference the scope refused")
		}
	})

	t.Run("enabled", func(t *testing.T) {
		expr, diags := hclsyntax.ParseExpression([]byte("aws_thing.r.on"), "test.hcl", hcl.InitialPos)
		if diags.HasErrors() {
			t.Fatalf("parsing: %s", diags.Error())
		}
		_, deferred, err := xlang.EvaluateEnabled(expr, scope)
		if deferred {
			t.Fatal("deferred on a scope that diagnosed the reference rather than deferring it")
		}
		if err == nil {
			t.Fatal("no error from an enabled whose reference the scope refused")
		}
	})
}

// The permissive side of the same rule: an unknown *value* still defers, which
// is how a walk waits for something genuinely not yet computed.
func TestRepetitionDefersOnAnUnknownValue(t *testing.T) {
	s := xlang.NewScope()
	s.SetVariable("n", cty.UnknownVal(cty.Number))
	s.SetVariable("each", cty.UnknownVal(cty.Set(cty.String)))

	countExpr, _ := hclsyntax.ParseExpression([]byte("var.n"), "test.hcl", hcl.InitialPos)
	if _, deferred, err := xlang.EvaluateCount(countExpr, s.EvalScope()); err != nil || !deferred {
		t.Fatalf("count over an unknown: deferred=%v err=%v", deferred, err)
	}

	eachExpr, _ := hclsyntax.ParseExpression([]byte("var.each"), "test.hcl", hcl.InitialPos)
	if _, deferred, err := xlang.EvaluateForEach(eachExpr, s.EvalScope(), false, true); err != nil || !deferred {
		t.Fatalf("for_each over an unknown: deferred=%v err=%v", deferred, err)
	}
}
