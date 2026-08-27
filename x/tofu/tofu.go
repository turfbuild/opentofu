// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

// Package tofu is a stable boundary over OpenTofu's internal tofu.Schemas type
// — the aggregate provider-schema set that jsonplan/jsonstate/jsonconfig
// marshalling requires.
package tofu

import (
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tofu"
)

// Schemas is the aggregate provider-schema set keyed by provider address, as
// consumed by jsonplan.Marshal.
type Schemas = tofu.Schemas

// Provider is the provider address used as the Schemas map key. It must match
// the ProviderAddr.Provider carried on the plan's resource changes.
type Provider = addrs.Provider

// IdentityAttribute describes one attribute of a resource type's identity
// schema — the provider-declared, permanently stable key for a remote object.
// The protocol has no nested form, so an identity is always this flat.
type IdentityAttribute struct {
	// Type is the attribute's value type.
	Type cty.Type
	// RequiredForImport and OptionalForImport record whether a caller must, or
	// may, supply this attribute when locating an object by its identity.
	RequiredForImport bool
	OptionalForImport bool
	// Description is the provider's prose for the attribute.
	Description string
}

// ResourceSchema is one resource type's configuration schema together with the
// identity schema and version stamps the provider declared for it.
//
// The versions are not decoration. jsonstate marshalling compares Version
// against the schema version recorded on each state object and fails the whole
// marshal when the two disagree, so a caller that leaves it zero can only
// render prior state for resource types whose provider is still at version 0.
// IdentityVersion is reported as identity_schema_version, wherever an identity
// value accompanies it.
//
// Identity is what lets a plan carry identity values faithfully: encoding and
// decoding a change's identity halves both prefer the declared type and fall
// back to inferring one from the serialized bytes, and the inferred type is not
// always the declared one — a null optional attribute infers as dynamic, a list
// as a tuple. Leaving it nil is how a resource type says it has no identity.
type ResourceSchema struct {
	// Block is the resource type's configuration schema.
	Block *configschema.Block
	// Version is the provider's state schema version for this resource type.
	Version int64
	// IdentityVersion is the provider's resource-identity schema version for
	// this resource type; zero when the type declares no identity.
	IdentityVersion int64
	// Identity is the resource type's identity schema keyed by attribute name,
	// or nil when the type declares no identity.
	Identity map[string]IdentityAttribute
}

// providerSchema converts to the internal per-schema representation.
func (rs ResourceSchema) providerSchema() providers.Schema {
	return providers.Schema{
		Block:                 rs.Block,
		Version:               rs.Version,
		IdentitySchemaVersion: rs.IdentityVersion,
		IdentitySchema:        rs.identitySchema(),
	}
}

// identitySchema builds the identity's object schema, or nil when the resource
// type declares no identity. It is an Object with single nesting rather than a
// Block because identity attributes are flat by construction.
func (rs ResourceSchema) identitySchema() *configschema.Object {
	if len(rs.Identity) == 0 {
		return nil
	}
	attrs := make(map[string]*configschema.Attribute, len(rs.Identity))
	for name, a := range rs.Identity {
		attrs[name] = &configschema.Attribute{
			Type:        a.Type,
			Required:    a.RequiredForImport,
			Optional:    a.OptionalForImport,
			Description: a.Description,
		}
	}
	return &configschema.Object{
		Attributes: attrs,
		Nesting:    configschema.NestingSingle,
	}
}

// NewSchemas returns an empty Schemas ready to receive providers via SetProvider.
func NewSchemas() *Schemas {
	return &tofu.Schemas{
		Providers:    make(map[addrs.Provider]providers.ProviderSchema),
		Provisioners: make(map[string]*configschema.Block),
	}
}

// ProviderSchemas is one provider's schema set, split the way
// providers.ProviderSchema itself splits it: the provider's own configuration
// block plus the three resource modes.
//
// It is a struct rather than a parameter list because the three mode maps have
// the same type, so positionally they are indistinguishable — and transposing
// two of them would not fail to compile, it would marshal one mode's
// configuration against another mode's schema.
type ProviderSchemas struct {
	// Config is the provider's own configuration schema. It is version-free:
	// only resource types carry a state schema version.
	Config *configschema.Block
	// Resources, DataSources and Ephemerals are keyed by type name.
	Resources   map[string]ResourceSchema
	DataSources map[string]ResourceSchema
	// Ephemerals must be populated for any configuration that declares an
	// `ephemeral` block: jsonconfig walks every resource in the module and
	// fails the whole marshal with "no schema found" for a mode it was not
	// given, so an omission here breaks plan export outright rather than
	// degrading it.
	Ephemerals map[string]ResourceSchema
}

// SetProvider stores a provider's schemas in s under addr, in the form
// jsonplan/jsonstate marshalling expects. It replaces any existing entry for
// addr.
func SetProvider(s *Schemas, addr Provider, schemas ProviderSchemas) {
	ps := providers.ProviderSchema{
		Provider:           providers.Schema{Block: schemas.Config},
		ResourceTypes:      make(map[string]providers.Schema, len(schemas.Resources)),
		DataSources:        make(map[string]providers.Schema, len(schemas.DataSources)),
		EphemeralResources: make(map[string]providers.Schema, len(schemas.Ephemerals)),
	}
	for name, rs := range schemas.Resources {
		ps.ResourceTypes[name] = rs.providerSchema()
	}
	for name, rs := range schemas.DataSources {
		ps.DataSources[name] = rs.providerSchema()
	}
	for name, rs := range schemas.Ephemerals {
		ps.EphemeralResources[name] = rs.providerSchema()
	}
	s.Providers[addr] = ps
}
