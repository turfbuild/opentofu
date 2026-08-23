// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zclconf/go-cty/cty"
)

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func evalString(t *testing.T, exprs map[string]Expression, name string) string {
	t.Helper()
	expr, ok := exprs[name]
	if !ok {
		t.Fatalf("variable %q not present in %v", name, exprs)
	}
	val, diags := expr.Value(nil)
	if diags.HasErrors() {
		t.Fatalf("evaluating %q: %s", name, diags.Error())
	}
	if val.Type() != cty.String {
		val, _ = val.Unmark()
		t.Fatalf("variable %q is %#v, want string", name, val)
	}
	return val.AsString()
}

// TestCollectAutoVarFiles pins OpenTofu's auto-load order: terraform.tfvars,
// terraform.tfvars.json, then every *.auto.tfvars / *.auto.tfvars.json in ONE
// combined lexical pass — a .json auto file sorts among the .tfvars ones by
// name, it does not trail them as a group.
func TestCollectAutoVarFiles(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"terraform.tfvars":      `x = "tfvars"`,
		"terraform.tfvars.json": `{"x": "tfvars-json"}`,
		"a.auto.tfvars.json":    `{"x": "a-json"}`,
		"b.auto.tfvars":         `x = "b"`,
		"c.auto.tfvars.json":    `{"x": "c-json"}`,
		"notavarfile.tfvars":    `x = "never"`, // not auto-loaded: neither default name nor *.auto.*
	})
	paths, err := CollectAutoVarFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range paths {
		names = append(names, filepath.Base(p))
	}
	want := []string{
		"terraform.tfvars",
		"terraform.tfvars.json",
		"a.auto.tfvars.json",
		"b.auto.tfvars",
		"c.auto.tfvars.json",
	}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("order mismatch:\n got %v\nwant %v", names, want)
	}
}

// TestAutoLoadVariablesFiles_JSONAndPrecedence proves .tfvars.json files load
// (they used to be skipped entirely) and that the merged map honors the
// collection order — the lexically-last auto file wins.
func TestAutoLoadVariablesFiles_JSONAndPrecedence(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"terraform.tfvars":      `x = "tfvars"` + "\n" + `only_hcl = "hcl"`,
		"terraform.tfvars.json": `{"x": "tfvars-json", "only_json": "json"}`,
		"a.auto.tfvars.json":    `{"x": "a-json"}`,
		"b.auto.tfvars":         `x = "b"`,
	})
	exprs, err := AutoLoadVariablesFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, exprs, "x"); got != "b" {
		t.Errorf("x = %q, want %q (lexically-last auto file wins)", got, "b")
	}
	if got := evalString(t, exprs, "only_hcl"); got != "hcl" {
		t.Errorf("only_hcl = %q, want %q", got, "hcl")
	}
	if got := evalString(t, exprs, "only_json"); got != "json" {
		t.Errorf("only_json = %q, want %q (JSON varfiles must load)", got, "json")
	}
}

// TestLoadVariablesFile_Syntax covers the per-file rules: JSON by extension,
// JSON by content sniff for ambiguous names, and the dedicated error for the
// declare-vs-assign mistake (`variable "x" {}` in a varfile).
func TestLoadVariablesFile_Syntax(t *testing.T) {
	t.Run("json by extension", func(t *testing.T) {
		dir := writeFiles(t, map[string]string{"prod.tfvars.json": `{"region": "us-east-1"}`})
		exprs, err := LoadVariablesFile(filepath.Join(dir, "prod.tfvars.json"))
		if err != nil {
			t.Fatal(err)
		}
		if got := evalString(t, exprs, "region"); got != "us-east-1" {
			t.Errorf("region = %q", got)
		}
	})

	t.Run("json by sniff for ambiguous name", func(t *testing.T) {
		dir := writeFiles(t, map[string]string{"vars": `  {"region": "eu-west-1"}`})
		exprs, err := LoadVariablesFile(filepath.Join(dir, "vars"))
		if err != nil {
			t.Fatal(err)
		}
		if got := evalString(t, exprs, "region"); got != "eu-west-1" {
			t.Errorf("region = %q", got)
		}
	})

	t.Run("variable block gets the declare-vs-assign error", func(t *testing.T) {
		dir := writeFiles(t, map[string]string{"oops.tfvars": "variable \"region\" {\n  default = \"x\"\n}\n"})
		_, err := LoadVariablesFile(filepath.Join(dir, "oops.tfvars"))
		if err == nil {
			t.Fatal("want error, got nil")
		}
		if !strings.Contains(err.Error(), "region = <value>") {
			t.Errorf("error should show the definition syntax, got: %v", err)
		}
	})

	t.Run("missing file surfaces the read error", func(t *testing.T) {
		if _, err := LoadVariablesFile(filepath.Join(t.TempDir(), "absent.tfvars")); err == nil {
			t.Fatal("want error, got nil")
		}
	})
}
