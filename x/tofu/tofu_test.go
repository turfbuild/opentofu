// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/zclconf/go-cty/cty"
)

func testBlock() *configschema.Block {
	return &configschema.Block{
		Attributes: map[string]*configschema.Attribute{
			"id": {Type: cty.String, Computed: true},
		},
	}
}

// TestSetProviderCarriesVersions pins the reason ResourceSchema exists: the
// versions a caller supplies must reach the accessor jsonstate marshalling
// consults, since jsonstate fails the marshal when the version it reads there
// disagrees with the one recorded on a state object.
func TestSetProviderCarriesVersions(t *testing.T) {
	addr := addrs.MustParseProviderSourceString("hashicorp/random")
	s := NewSchemas()
	SetProvider(s, addr, ProviderSchemas{
		Config: testBlock(),
		Resources: map[string]ResourceSchema{
			"random_password": {Block: testBlock(), Version: 3, IdentityVersion: 1},
			"random_pet":      {Block: testBlock()},
		},
		DataSources: map[string]ResourceSchema{
			"random_source": {Block: testBlock(), Version: 2},
		},
		Ephemerals: map[string]ResourceSchema{
			"random_password": {Block: testBlock()},
		},
	})

	schema, version := s.ResourceTypeConfig(addr, addrs.ManagedResourceMode, "random_password")
	if schema == nil {
		t.Fatal("no schema for random_password")
	}
	if version != 3 {
		t.Errorf("random_password schema version = %d, want 3", version)
	}
	if schema.IdentitySchemaVersion != 1 {
		t.Errorf("random_password identity schema version = %d, want 1", schema.IdentitySchemaVersion)
	}

	if _, version := s.ResourceTypeConfig(addr, addrs.ManagedResourceMode, "random_pet"); version != 0 {
		t.Errorf("random_pet schema version = %d, want 0", version)
	}

	// Data resources report version 0 whatever the provider declared: state for
	// them is discarded on every refresh, so SchemaForResourceType drops it.
	// Callers must therefore not stamp a version on data-source state either.
	dataSchema, dataVersion := s.ResourceTypeConfig(addr, addrs.DataResourceMode, "random_source")
	if dataSchema == nil {
		t.Fatal("no schema for data random_source")
	}
	if dataVersion != 0 {
		t.Errorf("data random_source reported version %d, want 0", dataVersion)
	}
	if dataSchema.Version != 2 {
		t.Errorf("data random_source stored version = %d, want 2", dataSchema.Version)
	}

	// The ephemeral entry deliberately reuses "random_password", a name the
	// managed map also holds: the three modes are separate namespaces, and a
	// lookup in one must not resolve out of another. Without this category
	// populated at all, jsonconfig fails the whole marshal with "no schema
	// found" for any configuration carrying an `ephemeral` block.
	ephSchema, ephVersion := s.ResourceTypeConfig(addr, addrs.EphemeralResourceMode, "random_password")
	if ephSchema == nil {
		t.Fatal("no schema for ephemeral random_password")
	}
	if ephVersion != 0 {
		t.Errorf("ephemeral random_password reported version %d, want 0", ephVersion)
	}
	if ephSchema.Version != 0 {
		t.Errorf("ephemeral random_password stored version = %d, want 0 (it resolved the managed entry)", ephSchema.Version)
	}
	if sch, _ := s.ResourceTypeConfig(addr, addrs.EphemeralResourceMode, "random_pet"); sch != nil {
		t.Error("random_pet is managed-only but resolved as an ephemeral type")
	}
}

// TestSetProviderReplaces documents that a second call for the same address
// replaces the first rather than merging into it.
func TestSetProviderReplaces(t *testing.T) {
	addr := addrs.MustParseProviderSourceString("hashicorp/random")
	s := NewSchemas()
	SetProvider(s, addr, ProviderSchemas{Config: testBlock(), Resources: map[string]ResourceSchema{
		"random_password": {Block: testBlock(), Version: 3},
	}})
	SetProvider(s, addr, ProviderSchemas{Config: testBlock(), Resources: map[string]ResourceSchema{
		"random_pet": {Block: testBlock()},
	}})

	ps := s.Providers[addr]
	if _, ok := ps.ResourceTypes["random_password"]; ok {
		t.Error("random_password survived a replacing SetProvider")
	}
	if _, ok := ps.ResourceTypes["random_pet"]; !ok {
		t.Error("random_pet missing after SetProvider")
	}
}

// TestSetProviderCarriesIdentitySchema pins what the identity half is for: a
// change's identity is serialized against the declared type, so the declared
// attributes must reach the schema marshalling consults. A resource type that
// declares no identity must leave it nil — that is how "no identity" travels.
func TestSetProviderCarriesIdentitySchema(t *testing.T) {
	addr := addrs.MustParseProviderSourceString("hashicorp/random")
	s := NewSchemas()
	SetProvider(s, addr, ProviderSchemas{Config: testBlock(), Resources: map[string]ResourceSchema{
		"random_password": {
			Block:           testBlock(),
			IdentityVersion: 1,
			Identity: map[string]IdentityAttribute{
				"id":   {Type: cty.String, RequiredForImport: true},
				"tags": {Type: cty.List(cty.String), OptionalForImport: true},
			},
		},
		"random_pet": {Block: testBlock()},
	}})

	schema, _ := s.ResourceTypeConfig(addr, addrs.ManagedResourceMode, "random_password")
	if schema == nil || schema.IdentitySchema == nil {
		t.Fatal("random_password carries no identity schema")
	}
	// The implied type is the load-bearing part: it is what the identity halves
	// of a change are encoded and decoded against, and inferring one from the
	// serialized bytes instead would read tags back as a tuple.
	want := cty.Object(map[string]cty.Type{
		"id":   cty.String,
		"tags": cty.List(cty.String),
	})
	if got := schema.IdentitySchema.ImpliedType(); !got.Equals(want) {
		t.Errorf("identity implied type = %#v, want %#v", got, want)
	}
	if attr := schema.IdentitySchema.Attributes["id"]; attr == nil || !attr.Required {
		t.Errorf("id should be a required identity attribute, got %#v", attr)
	}
	if attr := schema.IdentitySchema.Attributes["tags"]; attr == nil || !attr.Optional {
		t.Errorf("tags should be an optional identity attribute, got %#v", attr)
	}

	petSchema, _ := s.ResourceTypeConfig(addr, addrs.ManagedResourceMode, "random_pet")
	if petSchema == nil {
		t.Fatal("no schema for random_pet")
	}
	if petSchema.IdentitySchema != nil {
		t.Error("a resource type declaring no identity must report a nil identity schema")
	}
}
