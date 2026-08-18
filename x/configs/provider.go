// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
)

// LocalProviderConfig is a provider configuration named from inside one module:
// a local name plus an optional configuration alias.
type LocalProviderConfig = addrs.LocalProviderConfig

// ParseProviderConfigCompactStr parses a provider reference in the compact
// form a `provider =` meta-argument takes — "aws" or "aws.west" — into a
// module-local provider configuration address.
//
// Callers that hold an hcl.Expression should let the decoder produce a
// ProviderConfigRef instead; this is for the arguments that arrive as strings
// (MCP tool inputs, session bookkeeping), which have no source location to
// report against anyway.
//
// The first error from the underlying parser is returned as a regular Go error
// so callers don't need to import OpenTofu's tfdiags package.
//
// Extraneous traversal steps ("aws.west.extra") are rejected here rather than
// upstream: ParseProviderConfigCompact returns as soon as it reads the alias,
// so its own "extraneous extra operators" diagnostic is unreachable and a
// third step is silently dropped. A reference that means nothing should not
// resolve to the instance its prefix names.
func ParseProviderConfigCompactStr(str string) (LocalProviderConfig, error) {
	traversal, parseDiags := hclsyntax.ParseTraversalAbs([]byte(str), "", hcl.Pos{Line: 1, Column: 1})
	if parseDiags.HasErrors() {
		return LocalProviderConfig{}, parseDiags
	}
	if len(traversal) > 2 {
		return LocalProviderConfig{}, fmt.Errorf(
			"invalid provider configuration address %q: expected a provider type name, optionally followed by a period and a configuration alias", str)
	}
	addr, diags := configs.ParseProviderConfigCompact(traversal)
	if diags.HasErrors() {
		return LocalProviderConfig{}, diags.Err()
	}
	return addr, nil
}
