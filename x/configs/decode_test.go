// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"fmt"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/zclconf/go-cty/cty"
)

func parseBody(t *testing.T, src string) *hclsyntax.Body {
	t.Helper()
	file, diags := hclsyntax.ParseConfig([]byte(src), "test.tf", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("parse error: %s", diags.Error())
	}
	return file.Body.(*hclsyntax.Body)
}

func literalEvaluator(expr hcl.Expression) (any, error) {
	val, diags := expr.Value(nil)
	if diags.HasErrors() {
		return nil, fmt.Errorf("evaluation error: %s", diags.Error())
	}
	switch {
	case val.Type() == cty.String:
		return val.AsString(), nil
	case val.Type() == cty.Number:
		bf := val.AsBigFloat()
		if bf.IsInt() {
			i64, _ := bf.Int64()
			return i64, nil
		}
		f64, _ := bf.Float64()
		return f64, nil
	case val.Type() == cty.Bool:
		return val.True(), nil
	default:
		return val.GoString(), nil
	}
}

func TestDecodeBodyToConfig_Attributes(t *testing.T) {
	body := parseBody(t, `
		name   = "test"
		length = 3
		active = true
	`)

	schema := &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"name":   {Type: cty.String, Required: true},
			"length": {Type: cty.Number, Optional: true},
			"active": {Type: cty.Bool, Optional: true},
		},
	}

	config, err := decodeBody(body, schema, literalEvaluator)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config["name"] != "test" {
		t.Errorf("name = %v, want %q", config["name"], "test")
	}
	if config["length"] != int64(3) {
		t.Errorf("length = %v, want 3", config["length"])
	}
	if config["active"] != true {
		t.Errorf("active = %v, want true", config["active"])
	}
}

func TestDecodeBodyToConfig_NestingSingle(t *testing.T) {
	body := parseBody(t, `
		name = "parent"
		metadata {
			labels = "app"
		}
	`)

	schema := &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"name": {Type: cty.String, Required: true},
		},
		BlockTypes: map[string]*configschema.NestedBlock{
			"metadata": {
				Nesting: configschema.NestingSingle,
				Block: configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"labels": {Type: cty.String, Optional: true},
					},
				},
			},
		},
	}

	config, err := decodeBody(body, schema, literalEvaluator)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	metadata, ok := config["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata should be map[string]any, got %T", config["metadata"])
	}
	if metadata["labels"] != "app" {
		t.Errorf("metadata.labels = %v, want %q", metadata["labels"], "app")
	}
}

func TestDecodeBodyToConfig_NestingList(t *testing.T) {
	body := parseBody(t, `
		container {
			name  = "web"
			image = "nginx"
		}
		container {
			name  = "sidecar"
			image = "envoy"
		}
	`)

	schema := &configschema.Block{
		BlockTypes: map[string]*configschema.NestedBlock{
			"container": {
				Nesting: configschema.NestingList,
				Block: configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"name":  {Type: cty.String, Required: true},
						"image": {Type: cty.String, Required: true},
					},
				},
			},
		},
	}

	config, err := decodeBody(body, schema, literalEvaluator)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	containers, ok := config["container"].([]any)
	if !ok {
		t.Fatalf("container should be []any, got %T", config["container"])
	}
	if len(containers) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(containers))
	}

	c0, _ := containers[0].(map[string]any)
	if c0["name"] != "web" {
		t.Errorf("container[0].name = %v, want %q", c0["name"], "web")
	}
	c1, _ := containers[1].(map[string]any)
	if c1["name"] != "sidecar" {
		t.Errorf("container[1].name = %v, want %q", c1["name"], "sidecar")
	}
}

func TestDecodeBodyToConfig_NestingMap(t *testing.T) {
	body := parseBody(t, `
		port "http" {
			number = 80
		}
		port "https" {
			number = 443
		}
	`)

	schema := &configschema.Block{
		BlockTypes: map[string]*configschema.NestedBlock{
			"port": {
				Nesting: configschema.NestingMap,
				Block: configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"number": {Type: cty.Number, Required: true},
					},
				},
			},
		},
	}

	config, err := decodeBody(body, schema, literalEvaluator)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ports, ok := config["port"].(map[string]any)
	if !ok {
		t.Fatalf("port should be map[string]any, got %T", config["port"])
	}
	if len(ports) != 2 {
		t.Fatalf("expected 2 ports, got %d", len(ports))
	}

	http, _ := ports["http"].(map[string]any)
	if http["number"] != int64(80) {
		t.Errorf("port.http.number = %v, want 80", http["number"])
	}
	https, _ := ports["https"].(map[string]any)
	if https["number"] != int64(443) {
		t.Errorf("port.https.number = %v, want 443", https["number"])
	}
}

func TestDecodeBodyToConfig_NestedBlocks(t *testing.T) {
	// Test deeply nested blocks (e.g., Kubernetes-style metadata → labels)
	body := parseBody(t, `
		metadata {
			name      = "my-namespace"
			namespace = "default"
			labels {
				env = "production"
			}
		}
	`)

	schema := &configschema.Block{
		BlockTypes: map[string]*configschema.NestedBlock{
			"metadata": {
				Nesting: configschema.NestingSingle,
				Block: configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"name":      {Type: cty.String, Optional: true},
						"namespace": {Type: cty.String, Optional: true},
					},
					BlockTypes: map[string]*configschema.NestedBlock{
						"labels": {
							Nesting: configschema.NestingSingle,
							Block: configschema.Block{
								Attributes: map[string]*configschema.Attribute{
									"env": {Type: cty.String, Optional: true},
								},
							},
						},
					},
				},
			},
		},
	}

	config, err := decodeBody(body, schema, literalEvaluator)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	metadata, ok := config["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata should be map[string]any, got %T", config["metadata"])
	}
	if metadata["name"] != "my-namespace" {
		t.Errorf("metadata.name = %v, want %q", metadata["name"], "my-namespace")
	}

	labels, ok := metadata["labels"].(map[string]any)
	if !ok {
		t.Fatalf("metadata.labels should be map[string]any, got %T", metadata["labels"])
	}
	if labels["env"] != "production" {
		t.Errorf("metadata.labels.env = %v, want %q", labels["env"], "production")
	}
}

func TestDecodeBodyToConfig_UnknownValues(t *testing.T) {
	body := parseBody(t, `
		name   = some_resource.name
		length = 3
	`)

	schema := &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"name":   {Type: cty.String, Required: true},
			"length": {Type: cty.Number, Optional: true},
		},
	}

	// "name" references an unknown resource — should produce an error
	_, err := decodeBody(body, schema, literalEvaluator)
	if err == nil {
		t.Fatal("expected error for unresolved reference, got nil")
	}
}

func TestDecodeBodyToConfig_MissingAttributes(t *testing.T) {
	body := parseBody(t, `
		name = "test"
	`)

	schema := &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"name":     {Type: cty.String, Required: true},
			"optional": {Type: cty.String, Optional: true},
		},
	}

	config, err := decodeBody(body, schema, literalEvaluator)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config["name"] != "test" {
		t.Errorf("name = %v, want %q", config["name"], "test")
	}
	if _, exists := config["optional"]; exists {
		t.Error("optional attribute should not be in config when not specified in body")
	}
}

func TestDecodeBodyToConfig_EmptyBlock(t *testing.T) {
	body := parseBody(t, `
		features {}
	`)

	schema := &configschema.Block{
		BlockTypes: map[string]*configschema.NestedBlock{
			"features": {
				Nesting: configschema.NestingSingle,
				Block: configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"flag": {Type: cty.Bool, Optional: true},
					},
				},
			},
		},
	}

	config, err := decodeBody(body, schema, literalEvaluator)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	features, ok := config["features"].(map[string]any)
	if !ok {
		t.Fatalf("features should be map[string]any, got %T", config["features"])
	}
	// Empty block should produce an empty map
	if len(features) != 0 {
		t.Errorf("expected empty features map, got %v", features)
	}
}

// TestDecodeBodySensitivePaths verifies config-flow sensitivity capture: an
// attribute whose evaluated value carries the Sensitive mark yields a path
// (relative to the resource attribute object), and pvm.Path suffixes are
// preserved so a sensitive element inside a map attribute is located precisely.
func TestDecodeBodySensitivePaths(t *testing.T) {
	// Marked evaluator: literal value, marked Sensitive when the string value
	// (or a map element's string value) starts with "SEC".
	markSecrets := func(v cty.Value) cty.Value {
		if v.Type() == cty.String && !v.IsNull() {
			if s := v.AsString(); len(s) >= 3 && s[:3] == "SEC" {
				return v.Mark(marks.Sensitive)
			}
		}
		return v
	}
	eval := func(expr hcl.Expression) (cty.Value, error) {
		v, diags := expr.Value(nil)
		if diags.HasErrors() {
			return cty.NilVal, fmt.Errorf("%s", diags.Error())
		}
		if v.Type().IsObjectType() || v.Type().IsMapType() {
			m := map[string]cty.Value{}
			for k, ev := range v.AsValueMap() {
				m[k] = markSecrets(ev)
			}
			return cty.ObjectVal(m), nil
		}
		return markSecrets(v), nil
	}

	t.Run("top-level scalar attribute", func(t *testing.T) {
		body := parseBody(t, `
			name   = "plain"
			secret = "SEC-xyz"
		`)
		schema := &configschema.Block{
			Attributes: map[string]*configschema.Attribute{
				"name":   {Type: cty.String, Optional: true},
				"secret": {Type: cty.String, Optional: true},
			},
		}
		paths, err := DecodeBodySensitivePaths(body, schema, eval)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(paths) != 1 {
			t.Fatalf("want 1 path, got %d: %#v", len(paths), paths)
		}
		gp, ok := paths[0].Path[0].(cty.GetAttrStep)
		if !ok || gp.Name != "secret" {
			t.Errorf("want GetAttr{secret}, got %#v", paths[0].Path)
		}
	})

	t.Run("sensitive element inside a map attribute marks the whole attribute", func(t *testing.T) {
		// A `{...}` config literal is a cty object, but the provider returns
		// keepers coerced to its schema map type; a leaf path would not match, so
		// the whole top-level attribute is marked (safe over-redaction).
		body := parseBody(t, `
			keepers = { seed = "SEC-abc", note = "plain" }
		`)
		schema := &configschema.Block{
			Attributes: map[string]*configschema.Attribute{
				"keepers": {Type: cty.Map(cty.String), Optional: true},
			},
		}
		paths, err := DecodeBodySensitivePaths(body, schema, eval)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(paths) != 1 {
			t.Fatalf("want 1 path, got %d: %#v", len(paths), paths)
		}
		p := paths[0].Path
		if len(p) != 1 {
			t.Fatalf("want top-level-granular (1-step) path, got %#v", p)
		}
		if g, ok := p[0].(cty.GetAttrStep); !ok || g.Name != "keepers" {
			t.Errorf("want GetAttr{keepers}, got %#v", p)
		}
	})

	t.Run("nothing sensitive yields no paths", func(t *testing.T) {
		body := parseBody(t, `name = "plain"`)
		schema := &configschema.Block{
			Attributes: map[string]*configschema.Attribute{"name": {Type: cty.String, Optional: true}},
		}
		paths, err := DecodeBodySensitivePaths(body, schema, eval)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(paths) != 0 {
			t.Errorf("want no paths, got %#v", paths)
		}
	})
}

// TestDecodeBodyEphemeralPaths pins the Ephemeral twin: it finds the top-level
// elements whose config carries an Ephemeral mark (attribute directly, or
// buried in a nested block), ignores Sensitive-only values, and reports
// nothing on an unmarked body — the refusal gate must not fire spuriously.
func TestDecodeBodyEphemeralPaths(t *testing.T) {
	markByPrefix := func(v cty.Value) cty.Value {
		if v.Type() == cty.String && !v.IsNull() {
			switch s := v.AsString(); {
			case len(s) >= 3 && s[:3] == "EPH":
				return v.Mark(marks.Ephemeral)
			case len(s) >= 3 && s[:3] == "SEC":
				return v.Mark(marks.Sensitive)
			}
		}
		return v
	}
	eval := func(expr hcl.Expression) (cty.Value, error) {
		v, diags := expr.Value(nil)
		if diags.HasErrors() {
			return cty.NilVal, fmt.Errorf("%s", diags.Error())
		}
		return markByPrefix(v), nil
	}

	t.Run("ephemeral attribute found, sensitive ignored", func(t *testing.T) {
		body := parseBody(t, `
			name   = "plain"
			token  = "EPH-xyz"
			secret = "SEC-xyz"
		`)
		schema := &configschema.Block{
			Attributes: map[string]*configschema.Attribute{
				"name":   {Type: cty.String, Optional: true},
				"token":  {Type: cty.String, Optional: true},
				"secret": {Type: cty.String, Optional: true},
			},
		}
		paths, err := DecodeBodyEphemeralPaths(body, schema, eval)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(paths) != 1 {
			t.Fatalf("want 1 path, got %d: %#v", len(paths), paths)
		}
		if g, ok := paths[0][0].(cty.GetAttrStep); !ok || g.Name != "token" {
			t.Errorf("want GetAttr{token}, got %#v", paths[0])
		}
	})

	t.Run("ephemeral inside a nested block marks the block element", func(t *testing.T) {
		body := parseBody(t, `
			settings {
				password = "EPH-abc"
			}
		`)
		schema := &configschema.Block{
			BlockTypes: map[string]*configschema.NestedBlock{
				"settings": {
					Nesting: configschema.NestingSingle,
					Block: configschema.Block{
						Attributes: map[string]*configschema.Attribute{
							"password": {Type: cty.String, Optional: true},
						},
					},
				},
			},
		}
		paths, err := DecodeBodyEphemeralPaths(body, schema, eval)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(paths) != 1 {
			t.Fatalf("want 1 path, got %d: %#v", len(paths), paths)
		}
		if g, ok := paths[0][0].(cty.GetAttrStep); !ok || g.Name != "settings" {
			t.Errorf("want GetAttr{settings}, got %#v", paths[0])
		}
	})

	t.Run("unmarked body yields no paths", func(t *testing.T) {
		body := parseBody(t, `name = "plain"`)
		schema := &configschema.Block{
			Attributes: map[string]*configschema.Attribute{"name": {Type: cty.String, Optional: true}},
		}
		paths, err := DecodeBodyEphemeralPaths(body, schema, eval)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(paths) != 0 {
			t.Errorf("want no paths, got %#v", paths)
		}
	})
}

// TestExtractBodyToConfig covers the schema-less extractor used by config_init:
// nested blocks are preserved (a single block as an object, repeated blocks as a
// list, labeled blocks as a label-keyed map), attributes are evaluated, and a
// body containing only blocks is not dropped.
func TestExtractBodyToConfig(t *testing.T) {
	t.Run("single nested block surfaces as an object", func(t *testing.T) {
		body := parseBody(t, `
			resource_directory = "res"
			kubernetes {
				host  = "h"
				token = "t"
			}
		`)
		got, err := ExtractBodyToConfig(body, literalEvaluator)
		if err != nil {
			t.Fatal(err)
		}
		if got["resource_directory"] != "res" {
			t.Errorf("attribute lost: %#v", got)
		}
		kube, ok := got["kubernetes"].(map[string]any)
		if !ok {
			t.Fatalf("kubernetes not an object: %#v", got["kubernetes"])
		}
		if kube["host"] != "h" || kube["token"] != "t" {
			t.Errorf("nested block contents wrong: %#v", kube)
		}
	})

	t.Run("repeated blocks surface as a list", func(t *testing.T) {
		body := parseBody(t, `
			tag { key = "a" }
			tag { key = "b" }
		`)
		got, err := ExtractBodyToConfig(body, literalEvaluator)
		if err != nil {
			t.Fatal(err)
		}
		list, ok := got["tag"].([]any)
		if !ok || len(list) != 2 {
			t.Fatalf("tag not a 2-element list: %#v", got["tag"])
		}
		first := list[0].(map[string]any)
		second := list[1].(map[string]any)
		if first["key"] != "a" || second["key"] != "b" {
			t.Errorf("list order/contents wrong: %#v", list)
		}
	})

	t.Run("labeled blocks surface as a label-keyed map", func(t *testing.T) {
		body := parseBody(t, `
			endpoint "east" { url = "e" }
			endpoint "west" { url = "w" }
		`)
		got, err := ExtractBodyToConfig(body, literalEvaluator)
		if err != nil {
			t.Fatal(err)
		}
		m, ok := got["endpoint"].(map[string]any)
		if !ok {
			t.Fatalf("endpoint not a map: %#v", got["endpoint"])
		}
		if m["east"].(map[string]any)["url"] != "e" || m["west"].(map[string]any)["url"] != "w" {
			t.Errorf("labeled map wrong: %#v", m)
		}
	})

	t.Run("block-only body is not dropped", func(t *testing.T) {
		body := parseBody(t, `
			kubernetes { host = "h" }
		`)
		got, err := ExtractBodyToConfig(body, literalEvaluator)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := got["kubernetes"].(map[string]any); !ok {
			t.Fatalf("block-only body dropped: %#v", got)
		}
	})

	t.Run("deeply nested blocks recurse", func(t *testing.T) {
		body := parseBody(t, `
			exec {
				api_version = "v1"
				env { name = "X" }
			}
		`)
		got, err := ExtractBodyToConfig(body, literalEvaluator)
		if err != nil {
			t.Fatal(err)
		}
		exec := got["exec"].(map[string]any)
		if exec["api_version"] != "v1" {
			t.Errorf("nested attr lost: %#v", exec)
		}
		if exec["env"].(map[string]any)["name"] != "X" {
			t.Errorf("doubly-nested block lost: %#v", exec)
		}
	})
}

// The provider-block counterpart of the reference walker's rule: a meta-argument
// the decode lifted into its own field is not provider configuration, and
// handing one back is how `provider "random" { alias = "extra" }` came back as
// config {"alias": "extra"} and was rejected on the way to the plugin.
func TestExtractBodyToConfigIgnoresWhatTheDecodeConsumed(t *testing.T) {
	body := parseBody(t, `
		alias   = "west"
		version = "~> 3.0"
		region  = "us-west-2"
		assume_role {
			role_arn = "arn:aws:iam::1:role/r"
		}
	`)

	// What a provider decode consumes; see configs' providerBlockSchema.
	_, remain, diags := body.PartialContent(&hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "alias"}, {Name: "version"}, {Name: "for_each"},
			{Name: "count"}, {Name: "depends_on"}, {Name: "source"},
		},
	})
	if diags.HasErrors() {
		t.Fatalf("partial content: %s", diags.Error())
	}

	got, err := ExtractBodyToConfig(remain, literalEvaluator)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := got["alias"]; ok {
		t.Errorf("alias is a meta-argument, not provider config: %#v", got)
	}
	if _, ok := got["version"]; ok {
		t.Errorf("version is a meta-argument, not provider config: %#v", got)
	}
	if got["region"] != "us-west-2" {
		t.Errorf("real config lost: %#v", got)
	}
	if role, ok := got["assume_role"].(map[string]any); !ok || role["role_arn"] == nil {
		t.Errorf("nested config block lost: %#v", got["assume_role"])
	}
}
