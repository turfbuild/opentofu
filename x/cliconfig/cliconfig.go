// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

// Package cliconfig exposes the parts of OpenTofu's CLI configuration that a
// non-CLI host still needs: the host credentials that authenticate requests to
// registries and to the `remote`/`cloud` protocol backends.
//
// Credentials live outside any configuration directory — in `tofu login`'s
// credentials.tfrc.json, in `credentials` blocks in the CLI config file, or in
// TF_TOKEN_<host> environment variables — so a host that does not read the CLI
// config silently authenticates as nobody.
package cliconfig

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"time"

	"github.com/opentofu/svchost/disco"
	"github.com/opentofu/svchost/svcauth"

	"github.com/opentofu/opentofu/internal/command/cliconfig"
	"github.com/opentofu/opentofu/internal/httpclient"
	pluginDiscovery "github.com/opentofu/opentofu/internal/plugin/discovery"
)

// defaultRequestTimeout mirrors OpenTofu's own default
// (cliconfig.registryClientDefaultRequestTimeout). It applies only on the
// fallback path where no CLI configuration could be loaded; a loaded config
// always carries an explicit value.
const defaultRequestTimeout = 10 * time.Second

// NewServiceDiscovery builds a service-discovery object configured the way the
// tofu CLI configures its own: host credentials resolved from the CLI
// configuration (including credential helper programs and TF_TOKEN_<host>
// environment variables), over an HTTP client that retries and, crucially for
// a long-lived server, times out rather than hanging.
//
// Credentials that cannot be loaded are not fatal — anonymous discovery still
// works for public registries — so the returned error is advisory and the
// returned *disco.Disco is always usable.
func NewServiceDiscovery(ctx context.Context) (*disco.Disco, error) {
	config, diags := cliconfig.LoadConfig(ctx)
	if diags.HasErrors() {
		return anonymous(ctx, nil), fmt.Errorf("loading CLI configuration: %w", diags.Err())
	}

	credsSrc, err := credentialsSource(config)
	if err != nil {
		return anonymous(ctx, config), fmt.Errorf("initializing host credentials: %w", err)
	}
	return newDisco(ctx, config, credsSrc), nil
}

func anonymous(ctx context.Context, config *cliconfig.Config) *disco.Disco {
	// An untyped nil, so disco understands "no credentials available".
	return newDisco(ctx, config, nil)
}

func newDisco(ctx context.Context, config *cliconfig.Config, credsSrc svcauth.CredentialsSource) *disco.Disco {
	// For historical reasons the registry request retry policy also applies to
	// all service discovery requests; mirror cmd/tofu's newServiceDiscovery.
	var retryCount int
	var timeout = defaultRequestTimeout
	if config != nil && config.RegistryProtocols != nil {
		retryCount = config.RegistryProtocols.RetryCount
		timeout = config.RegistryProtocols.RequestTimeout
	}
	client := httpclient.NewForRegistryRequests(ctx, retryCount, timeout)
	return disco.New(
		disco.WithHTTPClient(client.HTTPClient),
		disco.WithCredentials(credsSrc),
	)
}

func credentialsSource(config *cliconfig.Config) (svcauth.CredentialsSource, error) {
	helperPlugins := pluginDiscovery.FindPlugins("credentials", globalPluginDirs())
	return config.CredentialsSource(helperPlugins)
}

// globalPluginDirs mirrors cmd/tofu's function of the same name; it is where
// credential helper programs are discovered. Restated here because cmd/tofu is
// a main package and cannot be imported.
func globalPluginDirs() []string {
	var ret []string
	dirs, err := cliconfig.DataDirs()
	if err != nil {
		log.Printf("[ERROR] Error finding global plugin directories: %s", err)
		return ret
	}
	machineDir := fmt.Sprintf("%s_%s", runtime.GOOS, runtime.GOARCH)
	for _, dir := range dirs {
		ret = append(ret, filepath.Join(dir, "plugins"))
		ret = append(ret, filepath.Join(dir, "plugins", machineDir))
	}
	return ret
}
