// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"strings"

	"github.com/opentofu/svchost"

	"github.com/opentofu/opentofu/internal/addrs"
)

// Provider is a fully-qualified provider address — hostname, namespace, and
// type ("registry.opentofu.org/hashicorp/aws"). It is the identity a provider
// is routed by at runtime and recorded under in state; a *local* name is only
// ever a per-module alias for one of these.
type Provider = addrs.Provider

// LocalProviderConfig is a provider configuration named from inside one
// module: a local name plus an optional configuration alias ("aws",
// "aws.west"). Translating one to a Provider requires that module's
// required_providers table — see configs.Module.ProviderForLocalConfig.
type LocalProviderConfig = addrs.LocalProviderConfig

// DefaultProviderRegistryHost is the hostname used for provider addresses that
// do not name one explicitly.
const DefaultProviderRegistryHost = addrs.DefaultProviderRegistryHost

// NewDefaultProvider returns the address of a provider under the default
// registry host and the "hashicorp" namespace.
var NewDefaultProvider = addrs.NewDefaultProvider

// ImpliedProviderForUnqualifiedType returns the provider address a bare type
// name implies when no required_providers entry claims it — the builtin
// "terraform" provider for that one name, and a default-registry hashicorp
// provider otherwise.
var ImpliedProviderForUnqualifiedType = addrs.ImpliedProviderForUnqualifiedType

// ParseProviderPart validates a single provider-address label (hostname
// segment, namespace, or type name) and returns it normalized.
var ParseProviderPart = addrs.ParseProviderPart

// NewProvider constructs a fully-qualified provider address. The hostname is
// taken as a string and normalized, so callers need not import svchost.
func NewProvider(hostname, namespace, typeName string) Provider {
	return addrs.NewProvider(svchost.Hostname(strings.ToLower(hostname)), namespace, typeName)
}

// ParseProviderSourceString parses a provider source string — "aws",
// "hashicorp/aws", or "registry.opentofu.org/hashicorp/aws" — into a
// fully-qualified address, validating every label. The first error from the
// underlying parser is returned as a regular Go error so callers don't need to
// import OpenTofu's tfdiags package.
func ParseProviderSourceString(str string) (Provider, error) {
	p, diags := addrs.ParseProviderSourceString(str)
	if diags.HasErrors() {
		return Provider{}, diags.Err()
	}
	return p, nil
}
