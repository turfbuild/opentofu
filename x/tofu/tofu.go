// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

// Package tofu is a stable boundary over OpenTofu's internal tofu.Schemas type
// — the aggregate provider-schema set that jsonplan/jsonstate/jsonconfig
// marshalling requires.
package tofu

import (
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

// ResourceSchema is one resource type's configuration schema together with the
// version stamps the provider declared for it.
//
// The versions are not decoration. jsonstate marshalling compares Version
// against the schema version recorded on each state object and fails the whole
// marshal when the two disagree, so a caller that leaves it zero can only
// render prior state for resource types whose provider is still at version 0.
// IdentityVersion is reported as identity_schema_version on planned values.
//
// The identity schema itself (providers.Schema.IdentitySchema) is deliberately
// not part of this type: marshalling reads the identity version but never the
// identity block, so carrying it would add a conversion no consumer reads.
type ResourceSchema struct {
	// Block is the resource type's configuration schema.
	Block *configschema.Block
	// Version is the provider's state schema version for this resource type.
	Version int64
	// IdentityVersion is the provider's resource-identity schema version for
	// this resource type; zero when the type declares no identity.
	IdentityVersion int64
}

// providerSchema converts to the internal per-schema representation.
func (rs ResourceSchema) providerSchema() providers.Schema {
	return providers.Schema{
		Block:                 rs.Block,
		Version:               rs.Version,
		IdentitySchemaVersion: rs.IdentityVersion,
	}
}

// NewSchemas returns an empty Schemas ready to receive providers via SetProvider.
func NewSchemas() *Schemas {
	return &tofu.Schemas{
		Providers:    make(map[addrs.Provider]providers.ProviderSchema),
		Provisioners: make(map[string]*configschema.Block),
	}
}

// SetProvider stores a provider's config/resource/data-source schemas in s
// under addr, in the form jsonplan/jsonstate marshalling expects. It replaces
// any existing entry for addr.
//
// The provider's own configuration schema is version-free: only resource types
// carry a state schema version.
func SetProvider(s *Schemas, addr Provider, config *configschema.Block, resources, dataSources map[string]ResourceSchema) {
	ps := providers.ProviderSchema{
		Provider:      providers.Schema{Block: config},
		ResourceTypes: make(map[string]providers.Schema, len(resources)),
		DataSources:   make(map[string]providers.Schema, len(dataSources)),
	}
	for name, rs := range resources {
		ps.ResourceTypes[name] = rs.providerSchema()
	}
	for name, rs := range dataSources {
		ps.DataSources[name] = rs.providerSchema()
	}
	s.Providers[addr] = ps
}
