// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package jsonplan_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/zclconf/go-cty/cty"

	xaddrs "github.com/opentofu/opentofu/x/addrs"
	xconfigs "github.com/opentofu/opentofu/x/configs"
	xjsonplan "github.com/opentofu/opentofu/x/jsonplan"
	xplans "github.com/opentofu/opentofu/x/plans"
	xtofu "github.com/opentofu/opentofu/x/tofu"
)

// TestMarshal renders a plan assembled entirely from facade types — a create
// and a deferred change — which is the property that makes a jsonplan-grade
// export reachable from outside this module, and pins the structured and
// encoded forms agreeing.
func TestMarshal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(`
resource "random_pet" "a" {
  length = 2
}
resource "random_pet" "b" {
  length = 3
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	loader, err := xconfigs.NewLoader(filepath.Join(dir, ".terraform", "modules"))
	if err != nil {
		t.Fatal(err)
	}
	config, err := loader.LoadConfig(context.Background(), dir, xconfigs.RootModuleCall(dir, "default", nil))
	if err != nil {
		t.Fatal(err)
	}

	provider := xaddrs.NewProvider("registry.opentofu.org", "hashicorp", "random")
	block := &xconfigs.Block{Attributes: map[string]*xconfigs.Attribute{
		"id":     {Type: cty.String, Computed: true},
		"length": {Type: cty.Number, Optional: true},
	}}
	schemas := xtofu.NewSchemas()
	xtofu.SetProvider(schemas, provider, xtofu.ProviderSchemas{
		Config:    &xconfigs.Block{},
		Resources: map[string]xtofu.ResourceSchema{"random_pet": {Block: block}},
	})

	ty := block.ImpliedType()
	change := func(name string, length int64, deferred string) *xplans.ResourceInstanceChangeSrc {
		t.Helper()
		addr, err := xaddrs.ParseAbsResourceInstance("random_pet." + name)
		if err != nil {
			t.Fatal(err)
		}
		before, err := xplans.NewDynamicValue(cty.NullVal(ty), ty)
		if err != nil {
			t.Fatal(err)
		}
		after, err := xplans.NewDynamicValue(cty.ObjectVal(map[string]cty.Value{
			"id":     cty.UnknownVal(cty.String),
			"length": cty.NumberIntVal(length),
		}), ty)
		if err != nil {
			t.Fatal(err)
		}
		return &xplans.ResourceInstanceChangeSrc{
			Addr:           addr,
			PrevRunAddr:    addr,
			ProviderAddr:   xaddrs.RootProviderConfig(provider),
			ChangeSrc:      xplans.ChangeSrc{Action: xplans.Create, Before: before, After: after},
			DeferredReason: deferred,
		}
	}

	changes := xplans.NewChanges()
	changes.Resources = append(changes.Resources, change("a", 2, ""), change("b", 3, "resource_config_unknown"))
	plan := &xplans.Plan{UIMode: xplans.NormalMode, Changes: changes}

	doc, err := xjsonplan.MarshalForLog(config, plan, nil, schemas)
	if err != nil {
		t.Fatal(err)
	}
	if doc.FormatVersion != xjsonplan.FormatVersion {
		t.Errorf("format_version = %q", doc.FormatVersion)
	}
	if len(doc.ResourceChanges) != 1 || doc.ResourceChanges[0].Address != "random_pet.a" {
		t.Fatalf("resource_changes = %+v, want random_pet.a alone", doc.ResourceChanges)
	}
	if len(doc.DeferredChanges) != 1 || doc.DeferredChanges[0].Reason != "resource_config_unknown" ||
		doc.DeferredChanges[0].ResourceChange.Address != "random_pet.b" {
		t.Fatalf("deferred_changes = %+v, want random_pet.b", doc.DeferredChanges)
	}

	raw, err := xjsonplan.Marshal(config, plan, nil, schemas)
	if err != nil {
		t.Fatal(err)
	}
	var back xjsonplan.Plan
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.ResourceChanges) != 1 || len(back.DeferredChanges) != 1 {
		t.Errorf("the encoded form disagrees with the structured one: %s", raw)
	}
}
