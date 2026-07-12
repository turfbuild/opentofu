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

// NewSchemas returns an empty Schemas ready to receive providers via SetProvider.
func NewSchemas() *Schemas {
	return &tofu.Schemas{
		Providers:    make(map[addrs.Provider]providers.ProviderSchema),
		Provisioners: make(map[string]*configschema.Block),
	}
}

// SetProvider stores a provider's config/resource/data-source schema blocks in
// s under addr, in the form jsonplan/jsonstate marshalling expects. It replaces
// any existing entry for addr.
func SetProvider(s *Schemas, addr Provider, config *configschema.Block, resources, dataSources map[string]*configschema.Block) {
	ps := providers.ProviderSchema{
		Provider:      providers.Schema{Block: config},
		ResourceTypes: make(map[string]providers.Schema, len(resources)),
		DataSources:   make(map[string]providers.Schema, len(dataSources)),
	}
	for name, block := range resources {
		ps.ResourceTypes[name] = providers.Schema{Block: block}
	}
	for name, block := range dataSources {
		ps.DataSources[name] = providers.Schema{Block: block}
	}
	s.Providers[addr] = ps
}
