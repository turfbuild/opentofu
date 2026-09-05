// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package lang_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	xaddrs "github.com/opentofu/opentofu/x/addrs"
	xconfigs "github.com/opentofu/opentofu/x/configs"
	xlang "github.com/opentofu/opentofu/x/lang"
)

// testData is a caller-supplied evaluation backend. Its whole point is to be
// defined in an external test package: if this compiles, lang.Data is
// implementable from outside the facade, which is the property the seam
// exists to provide.
type testData struct {
	vars      map[string]cty.Value
	resources map[string]cty.Value
}

var _ xlang.Data = (*testData)(nil)

func (d *testData) StaticValidateReferences(context.Context, []*xaddrs.Reference, xaddrs.Referenceable, xaddrs.Referenceable) xlang.Diagnostics {
	return nil
}

func (d *testData) GetInputVariable(_ context.Context, addr xaddrs.InputVariable, _ xlang.SourceRange) (cty.Value, xlang.Diagnostics) {
	if v, ok := d.vars[addr.Name]; ok {
		return v, nil
	}
	return cty.DynamicVal, nil
}

func (d *testData) GetResource(_ context.Context, addr xaddrs.Resource, _ xlang.SourceRange) (cty.Value, xlang.Diagnostics) {
	if v, ok := d.resources[addr.Type+"."+addr.Name]; ok {
		return v, nil
	}
	return cty.DynamicVal, nil
}

func (d *testData) GetLocalValue(context.Context, xaddrs.LocalValue, xlang.SourceRange) (cty.Value, xlang.Diagnostics) {
	return cty.DynamicVal, nil
}
func (d *testData) GetModule(context.Context, xaddrs.ModuleCall, xlang.SourceRange) (cty.Value, xlang.Diagnostics) {
	return cty.DynamicVal, nil
}
func (d *testData) GetCountAttr(context.Context, xaddrs.CountAttr, xlang.SourceRange) (cty.Value, xlang.Diagnostics) {
	return cty.DynamicVal, nil
}
func (d *testData) GetForEachAttr(context.Context, xaddrs.ForEachAttr, xlang.SourceRange) (cty.Value, xlang.Diagnostics) {
	return cty.DynamicVal, nil
}
func (d *testData) GetPathAttr(context.Context, xaddrs.PathAttr, xlang.SourceRange) (cty.Value, xlang.Diagnostics) {
	return cty.DynamicVal, nil
}
func (d *testData) GetTerraformAttr(context.Context, xaddrs.TerraformAttr, xlang.SourceRange) (cty.Value, xlang.Diagnostics) {
	return cty.DynamicVal, nil
}
func (d *testData) GetOutput(context.Context, xaddrs.OutputValue, xlang.SourceRange) (cty.Value, xlang.Diagnostics) {
	return cty.DynamicVal, nil
}
func (d *testData) GetCheckBlock(context.Context, xaddrs.Check, xlang.SourceRange) (cty.Value, xlang.Diagnostics) {
	return cty.DynamicVal, nil
}

func parseBody(t *testing.T, src string) hcl.Body {
	t.Helper()
	f, diags := hclsyntax.ParseConfig([]byte(src), "test.tf", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("parse: %s", diags.Error())
	}
	return f.Body
}

// TestSeamTypedDecode is the seam's reason for existing: a consumer outside
// this module drives OpenTofu's own evaluator through a schema-driven decode
// and gets back a cty.Value carrying the schema's types — no map[string]any,
// no type inference after the fact.
func TestSeamTypedDecode(t *testing.T) {
	schema := &xconfigs.Block{
		Attributes: map[string]*xconfigs.Attribute{
			"name":  {Type: cty.String, Required: true},
			"count": {Type: cty.Number, Optional: true},
			"tags":  {Type: cty.Map(cty.String), Optional: true},
		},
	}

	body := parseBody(t, `
      name  = "web-${var.env}"
      count = 3
      tags  = { owner = var.owner }
    `)

	scope := &xlang.EvalScope{
		Data: &testData{vars: map[string]cty.Value{
			"env":   cty.StringVal("prod"),
			"owner": cty.StringVal("platform"),
		}},
	}

	spec := schema.DecoderSpec()
	refs, diags := xlang.References(nil, hcldec.Variables(body, spec))
	if diags.HasErrors() {
		t.Fatalf("references: %s", diags.Err())
	}
	evalCtx, diags := scope.EvalContext(context.Background(), refs)
	if diags.HasErrors() {
		t.Fatalf("eval context: %s", diags.Err())
	}
	val, decDiags := hcldec.Decode(body, spec, evalCtx)
	if decDiags.HasErrors() {
		t.Fatalf("decode: %s", decDiags.Error())
	}

	if got := val.GetAttr("name").AsString(); got != "web-prod" {
		t.Errorf("name = %q, want %q", got, "web-prod")
	}
	// The type comes from the schema, not from inference over a Go value:
	// count decodes as cty.Number because the schema said so.
	if got := val.GetAttr("count").Type(); got != cty.Number {
		t.Errorf("count type = %s, want cty.Number", got.FriendlyName())
	}
	if got := val.GetAttr("tags").Type(); !got.Equals(cty.Map(cty.String)) {
		t.Errorf("tags type = %s, want map(string)", got.FriendlyName())
	}
}

// TestSeamUnknownPropagates checks that a reference the backend cannot resolve
// yet becomes an unknown value rather than an error — this is how "known after
// apply" has to behave, and a backend returning an error instead would break
// planning.
func TestSeamUnknownPropagates(t *testing.T) {
	body := parseBody(t, `name = aws_instance.web.id`)
	schema := &xconfigs.Block{
		Attributes: map[string]*xconfigs.Attribute{"name": {Type: cty.String, Required: true}},
	}

	scope := &xlang.EvalScope{Data: &testData{}}
	spec := schema.DecoderSpec()
	refs, _ := xlang.References(nil, hcldec.Variables(body, spec))
	evalCtx, diags := scope.EvalContext(context.Background(), refs)
	if diags.HasErrors() {
		t.Fatalf("eval context: %s", diags.Err())
	}
	val, decDiags := hcldec.Decode(body, spec, evalCtx)
	if decDiags.HasErrors() {
		t.Fatalf("decode: %s", decDiags.Error())
	}
	if val.GetAttr("name").IsKnown() {
		t.Error("expected an unknown value for an unresolved reference")
	}
}

// TestSeamBaseDir pins the setting that the closed Scope could not express:
// file() resolves against the scope's BaseDir, not the process working
// directory. A server evaluating many modules must be able to set this.
func TestSeamBaseDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "greeting.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	expr, diags := hclsyntax.ParseExpression([]byte(`file("greeting.txt")`), "test.tf", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("parse: %s", diags.Error())
	}

	scope := &xlang.EvalScope{Data: &testData{}, BaseDir: dir}
	val, evalDiags := scope.EvalExpr(context.Background(), expr, cty.String)
	if evalDiags.HasErrors() {
		t.Fatalf("eval: %s", evalDiags.Err())
	}
	if got := val.AsString(); got != "hello" {
		t.Errorf("file() = %q, want %q", got, "hello")
	}
}

// TestSeamPureOnly pins the other setting the closed Scope could not express:
// during plan, an impure function must yield unknown rather than baking in a
// value that would differ at apply.
func TestSeamPureOnly(t *testing.T) {
	expr, diags := hclsyntax.ParseExpression([]byte(`uuid()`), "test.tf", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("parse: %s", diags.Error())
	}

	scope := &xlang.EvalScope{Data: &testData{}, PureOnly: true}
	val, evalDiags := scope.EvalExpr(context.Background(), expr, cty.String)
	if evalDiags.HasErrors() {
		t.Fatalf("eval: %s", evalDiags.Err())
	}
	if val.IsKnown() {
		t.Error("uuid() should be unknown under PureOnly")
	}
}

// TestSeamCaller pins the one reference a Data implementation cannot serve.
// `caller` aliases the triggering resource instance inside an action's
// configuration; it is not an address, so it is carried on the scope rather
// than looked up. The closed Scope grew a field for it, and a consumer
// evaluating an action_trigger through the seam needs the same one — without
// it the seam is strictly less capable than the scope it exists to generalize.
func TestSeamCaller(t *testing.T) {
	expr, diags := hclsyntax.ParseExpression([]byte(`caller.id`), "test.tf", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("parse: %s", diags.Error())
	}

	caller := cty.ObjectVal(map[string]cty.Value{"id": cty.StringVal("i-abc123")})
	scope := &xlang.EvalScope{Data: &testData{}, Caller: caller}
	val, evalDiags := scope.EvalExpr(context.Background(), expr, cty.String)
	if evalDiags.HasErrors() {
		t.Fatalf("eval: %s", evalDiags.Err())
	}
	if got := val.AsString(); got != "i-abc123" {
		t.Errorf("caller.id = %q, want %q", got, "i-abc123")
	}
}

// And a scope that was given no caller must refuse the reference rather than
// resolve it to null: outside an action_trigger there is no triggering
// instance, and a silent null would read as one with no attributes.
func TestSeamCallerUnavailableWithoutOne(t *testing.T) {
	expr, diags := hclsyntax.ParseExpression([]byte(`caller.id`), "test.tf", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("parse: %s", diags.Error())
	}

	scope := &xlang.EvalScope{Data: &testData{}}
	_, evalDiags := scope.EvalExpr(context.Background(), expr, cty.String)
	if !evalDiags.HasErrors() {
		t.Fatal("a scope with no caller resolved `caller` anyway")
	}
}
