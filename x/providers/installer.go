// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

// Package providers wraps OpenTofu's provider installation machinery
// (getproviders + providercache) so consumers resolve and install provider
// plugins the same way `tofu init` does: honoring the registry protocol,
// verifying checksums and signatures, and reusing a shared plugin cache via
// TF_PLUGIN_CACHE_DIR.
//
// This package lives under the github.com/opentofu/opentofu/x module path so
// that Go's internal-package rule permits importing github.com/opentofu/
// opentofu/internal/... — the same trick used by x/configs for modules.
package providers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/depsfile"
	"github.com/opentofu/opentofu/internal/getproviders"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/providercache"
	"github.com/opentofu/svchost/disco"
)

// defaultProviderDownloadRetries mirrors OpenTofu's default for transient
// registry/download retries (cliconfig.providerDownloadDefaultRetry). Hardcoded
// here to avoid pulling the heavy internal/command/cliconfig package into the
// facade just for one constant.
const defaultProviderDownloadRetries = 2

// Installer resolves version constraints and installs OpenTofu provider plugins
// into a local cache directory, returning the path to the runnable plugin
// binary. It wraps providercache.Installer over a memoized registry source.
type Installer struct {
	dir       *providercache.Dir
	installer *providercache.Installer

	// mu serializes EnsureProvider. providercache's target Dir is single-writer:
	// concurrent installs into the same directory race on mkdir. A consumer may load
	// providers from multiple goroutines, so guard the whole resolve+install.
	mu sync.Mutex
}

// NewInstaller builds an Installer that unpacks providers into localCacheDir.
//
// services carries the host credentials used to reach a provider registry; pass
// the same object the rest of the host uses (see x/cliconfig.NewServiceDiscovery)
// so a private registry authenticates. A nil services falls back to anonymous
// discovery, which reaches public registries only.
//
// If globalCacheDir is non-empty (e.g. from TF_PLUGIN_CACHE_DIR), it is used as
// a shared read-through cache: a provider already present there is linked into
// localCacheDir without re-downloading, so binaries are reused across server
// processes and across test runs. globalCacheDir uses OpenTofu's canonical
// plugin-cache layout, so it interoperates with a cache populated by `tofu`.
func NewInstaller(ctx context.Context, localCacheDir, globalCacheDir string, services *disco.Disco) (*Installer, error) {
	if localCacheDir == "" {
		return nil, fmt.Errorf("local cache dir must not be empty")
	}
	if err := os.MkdirAll(localCacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating provider cache dir: %w", err)
	}

	// A registry-tuned HTTP client (retry policy, user agent) shared by both
	// service discovery and the registry source, mirroring `tofu init`.
	httpClient := httpclient.NewForRegistryRequests(ctx, defaultProviderDownloadRetries, 0)
	if services == nil {
		services = disco.New(disco.WithHTTPClient(httpClient.HTTPClient))
	}

	var src getproviders.Source = getproviders.NewRegistrySource(ctx, services, httpClient, getproviders.LocationConfig{
		ProviderDownloadRetries: defaultProviderDownloadRetries,
	})
	// Optional local override: when TF_PROVIDER_MIRROR_DIR points at an
	// OpenTofu filesystem-mirror layout, a provider present in the mirror is
	// served from the mirror ONLY — the registry tier excludes it, the same
	// split a CLI-config `provider_installation` block expresses. Exclusion
	// rather than preference-with-fallback, for two reasons: a provider host
	// that exists only in the mirror must not consult the registry at all
	// (MultiSource propagates a host-resolution failure even when another
	// tier answered, so the fallback poisons the lookup); and a silent
	// fallback to the registry's build of a mirrored provider is exactly the
	// wrong-provider hazard a test mirror exists to prevent. Providers not in
	// the mirror resolve from the registry as always. Off by default — the
	// env var is unset in production, leaving registry-only behavior
	// unchanged. Used by tests to load a locally-built provider (e.g. a fork
	// with failure injection).
	if mirrorDir := os.Getenv("TF_PROVIDER_MIRROR_DIR"); mirrorDir != "" {
		exclude, err := getproviders.ParseMultiSourceMatchingPatterns(mirrorProviderPatterns(mirrorDir))
		if err != nil {
			return nil, fmt.Errorf("scanning provider mirror %s: %w", mirrorDir, err)
		}
		src = getproviders.MultiSource{
			{Source: getproviders.NewFilesystemMirrorSource(ctx, mirrorDir)},
			{Source: src, Exclude: exclude},
		}
	}
	source := getproviders.NewMemoizeSource(src)

	dir := providercache.NewDir(localCacheDir)
	inst := providercache.NewInstaller(dir, source)
	if globalCacheDir != "" {
		if err := os.MkdirAll(globalCacheDir, 0o755); err != nil {
			return nil, fmt.Errorf("creating global plugin cache dir: %w", err)
		}
		inst.SetGlobalCacheDir(providercache.NewDir(globalCacheDir))
		// No dependency lock file is maintained here, so allow populating the
		// global cache from packages whose hashes aren't already locked.
		inst.SetGlobalCacheDirMayBreakDependencyLockFile(true)
	}

	return &Installer{dir: dir, installer: inst}, nil
}

// mirrorProviderPatterns lists the providers a filesystem mirror carries, as
// "hostname/namespace/type" matching patterns — the exclusion set for the
// registry tier. The scan is the mirror layout's own shape: three directory
// levels, anything that is not a directory ignored. An unreadable or empty
// mirror yields no patterns, leaving the registry tier unrestricted.
func mirrorProviderPatterns(mirrorDir string) []string {
	var patterns []string
	hosts, err := os.ReadDir(mirrorDir)
	if err != nil {
		return nil
	}
	for _, host := range hosts {
		if !host.IsDir() {
			continue
		}
		namespaces, err := os.ReadDir(filepath.Join(mirrorDir, host.Name()))
		if err != nil {
			continue
		}
		for _, ns := range namespaces {
			if !ns.IsDir() {
				continue
			}
			types, err := os.ReadDir(filepath.Join(mirrorDir, host.Name(), ns.Name()))
			if err != nil {
				continue
			}
			for _, typ := range types {
				if !typ.IsDir() {
					continue
				}
				patterns = append(patterns, host.Name()+"/"+ns.Name()+"/"+typ.Name())
			}
		}
	}
	return patterns
}

// EnsureProvider resolves versionConstraint against the registry, installs the
// selected version into the cache (verifying its checksum), and returns the
// path to the runnable plugin binary along with the resolved version string.
//
// source is an OpenTofu provider source address ("namespace/name" or
// "hostname/namespace/name"); the default host is registry.opentofu.org. An
// empty versionConstraint means "latest".
func (i *Installer) EnsureProvider(ctx context.Context, source, versionConstraint string) (binaryPath, resolvedVersion string, err error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	addr, diags := addrs.ParseProviderSourceString(source)
	if diags.HasErrors() {
		return "", "", fmt.Errorf("parsing provider source %q: %w", source, diags.Err())
	}

	versionConstraint = strings.TrimSpace(versionConstraint)
	switch {
	case versionConstraint == "":
		versionConstraint = ">= 0"
	case versionConstraint[0] == 'v' && len(versionConstraint) > 1 && versionConstraint[1] >= '0' && versionConstraint[1] <= '9':
		// Tolerate a "v"-prefixed exact version (e.g. "v3.9.0"); getproviders'
		// constraint parser rejects the leading "v".
		versionConstraint = versionConstraint[1:]
	}
	constraints, err := getproviders.ParseVersionConstraints(versionConstraint)
	if err != nil {
		return "", "", fmt.Errorf("parsing version constraint %q: %w", versionConstraint, err)
	}

	reqs := getproviders.Requirements{addr: constraints}
	locks, err := i.installer.EnsureProviderVersions(ctx, depsfile.NewLocks(), reqs, providercache.InstallNewProvidersOnly)
	if err != nil {
		return "", "", fmt.Errorf("installing provider %s: %w", addr, err)
	}

	lock := locks.Provider(addr)
	if lock == nil {
		return "", "", fmt.Errorf("provider %s was not installed", addr)
	}
	version := lock.Version()

	cached := i.dir.ProviderVersion(addr, version)
	if cached == nil {
		return "", "", fmt.Errorf("provider %s %s not found in cache after install", addr, version)
	}
	binary, err := cached.ExecutableFile()
	if err != nil {
		return "", "", fmt.Errorf("locating provider binary for %s %s: %w", addr, version, err)
	}
	return binary, version.String(), nil
}
