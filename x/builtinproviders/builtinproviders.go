// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

// Package builtinproviders serves OpenTofu's in-process ("built-in")
// providers as ordinary go-plugin provider servers, so that a host which
// consumes OpenTofu as a library and speaks only the plugin protocol can run
// terraform_data.
//
// The CLI never launches a built-in provider: Meta.internalProviders
// (internal/command/meta_providers.go) hands core an in-process factory, and
// the installer is told to skip the type. A library host that talks to every
// provider over the wire has no such path — the provider lives behind the
// internal-package boundary and no registry serves it. This package is the
// missing half: the set of built-in providers, their addresses, and Serve,
// which turns the calling process into a plugin server for one of them. The
// serving shape mirrors internal/provider-simple/main: a small binary linking
// this package calls Serve with the provider's name, and the host's plugin
// client launches it exactly as it would a downloaded provider.
//
// That binary is its own when the host's plugin client is typed against a
// different generated tfplugin5 package than this module's (provider-client's,
// say): both register the same proto names, and a process linking both panics
// at init. So the server links this package and the client does not, and the
// two meet over the plugin handshake like any provider and any client.
//
// Not covered: terraform_remote_state. Core reads it past the provider
// interface (ReadDataSourceEncrypted, with the state encryption it holds), and
// the provider's own ReadDataSource panics if reached — over the wire it would
// kill the child. A host that needs it has to build the data source itself.
package builtinproviders

import (
	"fmt"
	"sort"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/builtin/providers/tf"
	"github.com/opentofu/opentofu/internal/grpcwrap"
	"github.com/opentofu/opentofu/internal/plugin"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfplugin5"
)

// factories is the set Meta.internalProviders builds, keyed by the type name
// that addrs.NewBuiltInProvider takes.
var factories = map[string]providers.Factory{
	"terraform": func() (providers.Interface, error) {
		return tf.NewProvider(), nil
	},
}

// Names lists the built-in provider type names this package can serve, in a
// stable order.
func Names() []string {
	names := make([]string, 0, len(factories))
	for name := range factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Addr returns the fully-qualified address of a built-in provider —
// "terraform.io/builtin/<name>" — which is how a configuration resolves the
// bare "terraform" local name (addrs.ImpliedProviderForUnqualifiedType) and
// how a state record spells the provider. It does not check that the name is
// one this package serves; Names does.
func Addr(name string) addrs.Provider {
	return addrs.NewBuiltInProvider(name)
}

// Serve runs the named built-in provider as a go-plugin provider server over
// this process's stdio, the way a provider binary does when a plugin client
// launches it. It blocks until the client disconnects and must be the last
// thing the process does: go-plugin owns stdout for the handshake, so nothing
// may write there before it. The only way Serve returns is an unknown name,
// reported before the handshake begins.
func Serve(name string) error {
	server, err := serverFor(name)
	if err != nil {
		return err
	}
	plugin.Serve(&plugin.ServeOpts{
		GRPCProviderFunc: func() tfplugin5.ProviderServer {
			return server
		},
	})
	return nil
}

// serverFor wraps a built-in provider as a protocol-5 gRPC server — the same
// wrapping the provider-simple test binary uses. Unexported: tfplugin5 is an
// internal type, and a facade consumer needs Serve, not the server.
func serverFor(name string) (tfplugin5.ProviderServer, error) {
	factory, ok := factories[name]
	if !ok {
		return nil, fmt.Errorf("no built-in provider named %q (this build serves %v)", name, Names())
	}
	p, err := factory()
	if err != nil {
		return nil, fmt.Errorf("built-in provider %q: %w", name, err)
	}
	return grpcwrap.Provider(p), nil
}
