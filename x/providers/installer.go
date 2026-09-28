// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

// Package providers wraps OpenTofu's provider installation machinery
// (getproviders + providercache) so consumers resolve and install provider
// plugins the same way `tofu init` does: from the sources the CLI
// configuration's provider_installation block names (or the implied local
// mirrors and the registry when it names none), verifying checksums and
// signatures, and reusing a shared plugin cache.
//
// This package lives under the github.com/opentofu/opentofu/x module path so
// that Go's internal-package rule permits importing github.com/opentofu/
// opentofu/internal/... — the same trick used by x/configs for modules.
package providers

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/apparentlymart/go-userdirs/userdirs"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/opentofu/svchost/disco"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command/cliconfig"
	"github.com/opentofu/opentofu/internal/depsfile"
	"github.com/opentofu/opentofu/internal/getproviders"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/oci"
	"github.com/opentofu/opentofu/internal/providercache"
)

// Installer resolves version constraints and installs OpenTofu provider plugins
// into a local cache directory, returning the path to the runnable plugin
// binary. It wraps providercache.Installer over the source the CLI
// configuration describes.
type Installer struct {
	dir       *providercache.Dir
	installer *providercache.Installer
	// source is what installer installs from, kept for the package's own
	// tests to inspect.
	source getproviders.Source

	// overrides are the CLI configuration's dev_overrides: providers a host
	// launches from a local directory instead of an installed package.
	overrides map[addrs.Provider]getproviders.PackageLocalDir

	// mu serializes EnsureProvider. providercache's target Dir is single-writer:
	// concurrent installs into the same directory race on mkdir. A consumer may load
	// providers from multiple goroutines, so guard the whole resolve+install.
	mu sync.Mutex
}

// NewInstaller builds an Installer that unpacks providers into localCacheDir.
//
// Providers come from where `tofu init` would take them: the CLI
// configuration's provider_installation block (`~/.tofurc`,
// `~/.terraform.d/*.tfrc`, or the one file TF_CLI_CONFIG_FILE names), its
// direct, filesystem_mirror, network_mirror and oci_mirror methods in order,
// each with its include and exclude patterns; or, with no block, the implied
// local mirrors (see impliedProviderSource) ahead of the registry. The
// configuration is read once, here.
//
// A configuration in error is refused, where tofu reports it and carries on
// because most of its commands install nothing. A host installs on every
// plan, and what failed to load may be the very block that names a mirror: a
// method that does not decode is dropped from the block, and a file that does
// not parse loses all of it, so installing from what remains could fetch from
// the registry what the operator meant to mirror. For the same reason a
// TF_CLI_CONFIG_FILE (or TERRAFORM_CONFIG) naming a file that cannot be
// opened is refused, where tofu warns.
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
// The CLI configuration's plugin_cache_dir is not read; the caller decides.
func NewInstaller(ctx context.Context, localCacheDir, globalCacheDir string, services *disco.Disco) (*Installer, error) {
	if localCacheDir == "" {
		return nil, fmt.Errorf("local cache dir must not be empty")
	}
	if err := os.MkdirAll(localCacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating provider cache dir: %w", err)
	}

	config, diags := cliconfig.LoadConfig(ctx)
	if diags.HasErrors() {
		return nil, fmt.Errorf("the CLI configuration is in error: %w", diags.Err())
	}
	if file := cliConfigFileOverride(); file != "" {
		// LoadConfig only warns, and goes on without the file.
		f, err := os.Open(file)
		if err != nil {
			return nil, fmt.Errorf("the CLI configuration file the environment names cannot be opened: %w", err)
		}
		_ = f.Close()
	}

	if services == nil {
		services = disco.New(disco.WithHTTPClient(registryHTTPClient(ctx, config.RegistryProtocols).HTTPClient))
	}
	source, err := providerSource(ctx, config, services)
	if err != nil {
		return nil, err
	}

	dir := providercache.NewDir(localCacheDir)
	inst := providercache.NewInstaller(dir, source)
	if globalCacheDir != "" {
		if err := os.MkdirAll(globalCacheDir, 0o755); err != nil {
			return nil, fmt.Errorf("creating global plugin cache dir: %w", err)
		}
		inst.SetGlobalCacheDir(providercache.NewDir(globalCacheDir))
		// A caller that keeps no dependency lock file has no checksums to
		// confirm a cached package against, so allow linking from the global
		// cache regardless; SetPluginCacheMayBreakLockFile turns it off.
		inst.SetGlobalCacheDirMayBreakDependencyLockFile(true)
	}

	i := &Installer{dir: dir, installer: inst, source: source}
	if len(config.ProviderInstallation) > 0 {
		i.overrides = config.ProviderInstallation[0].DevOverrides
	}
	for _, addr := range i.DevOverrides() {
		log.Printf("[WARN] Provider development overrides are in effect: %s launches from %s", addr, i.overrides[addr])
	}
	return i, nil
}

// cliConfigFileOverride is internal/command/cliconfig's function of the same
// name: the one CLI configuration file the environment names, if any.
func cliConfigFileOverride() string {
	if file := os.Getenv("TF_CLI_CONFIG_FILE"); file != "" {
		return file
	}
	return os.Getenv("TERRAFORM_CONFIG")
}

// SetPluginCacheMayBreakLockFile is the CLI configuration's
// plugin_cache_may_break_dependency_lock_file. NewInstaller sets it, for a
// caller that keeps no lock file. A caller that writes one turns it off, as
// `tofu init` has it by default: a package is then linked from the global
// cache only when locked checksums confirm it, and otherwise fetched from its
// origin, so the lock file records the origin's checksums for every
// platform rather than one local checksum another platform cannot match.
func (i *Installer) SetPluginCacheMayBreakLockFile(mayBreak bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.installer.SetGlobalCacheDirMayBreakDependencyLockFile(mayBreak)
}

// DevOverrides lists the providers the CLI configuration's dev_overrides
// name, sorted. As under tofu, installing one is unaffected: `tofu init`
// installs and locks the published version and only warns. A host substitutes
// the override when it launches the provider (DevOverride), and passes the
// lock-file check for it by marking it overridden in the locks
// (depsfile.Locks.SetProviderOverridden), as tofu's plan and apply do.
func (i *Installer) DevOverrides() []addrs.Provider {
	ret := make([]addrs.Provider, 0, len(i.overrides))
	for addr := range i.overrides {
		ret = append(ret, addr)
	}
	sort.Slice(ret, func(a, b int) bool { return ret[a].LessThan(ret[b]) })
	return ret
}

// DevOverride reports the CLI configuration's dev_overrides entry for a
// provider source address, if it has one, as the package a host launches in
// place of an installed one: the override directory's plugin executable, with
// no version, since an overridden provider is not versioned. It mirrors
// internal/command's devOverrideProviderFactory. The directory is not
// installed, copied or checksummed. A relative override path resolves, as
// under tofu, against the process's working directory, at each call, and so
// does the Binary it yields.
func (i *Installer) DevOverride(source string) (ProviderFacts, bool, error) {
	addr, diags := addrs.ParseProviderSourceString(source)
	if diags.HasErrors() {
		return ProviderFacts{}, false, fmt.Errorf("parsing provider source %q: %w", source, diags.Err())
	}
	dir, ok := i.overrides[addr]
	if !ok {
		return ProviderFacts{}, false, nil
	}
	cached := providercache.CachedProvider{
		Provider:   addr,
		Version:    getproviders.UnspecifiedVersion,
		PackageDir: string(dir),
	}
	binary, err := cached.ExecutableFile()
	if err != nil {
		return ProviderFacts{}, true, fmt.Errorf("provider %s is overridden by dev_overrides to load from %s: %w", addr, dir, err)
	}
	return ProviderFacts{
		Provider:   addr.String(),
		Binary:     binary,
		PackageDir: string(dir),
	}, true, nil
}

// providerSource restates cmd/tofu's providerSource and
// explicitProviderSource (cmd/tofu/provider_source.go), which a library host
// cannot import: the provider_installation block's methods in order, each
// with its include and exclude patterns, or with no block the implied source.
// tofu reports a method it cannot build and carries on without it; a host
// refuses the configuration, for the reason NewInstaller gives.
func providerSource(ctx context.Context, config *cliconfig.Config, services *disco.Disco) (getproviders.Source, error) {
	if len(config.ProviderInstallation) == 0 {
		return impliedProviderSource(ctx, config.RegistryProtocols, services), nil
	}
	// The CLI configuration's validation admits at most one block.
	var rules getproviders.MultiSource
	for _, method := range config.ProviderInstallation[0].Methods {
		source, err := methodSource(ctx, method, config, services)
		if err != nil {
			return nil, fmt.Errorf("provider_installation: %w", err)
		}
		include, err := getproviders.ParseMultiSourceMatchingPatterns(method.Include)
		if err != nil {
			return nil, fmt.Errorf("provider_installation: invalid provider inclusion patterns: %w", err)
		}
		exclude, err := getproviders.ParseMultiSourceMatchingPatterns(method.Exclude)
		if err != nil {
			return nil, fmt.Errorf("provider_installation: invalid provider exclusion patterns: %w", err)
		}
		rules = append(rules, getproviders.MultiSourceSelector{Source: source, Include: include, Exclude: exclude})
	}
	return rules, nil
}

// methodSource restates cmd/tofu's providerSourceForCLIConfigLocation: the
// source one installation method names.
func methodSource(ctx context.Context, method *cliconfig.ProviderInstallationMethod, config *cliconfig.Config, services *disco.Disco) (getproviders.Source, error) {
	if method.Location == cliconfig.ProviderInstallationDirect {
		return getproviders.NewMemoizeSource(
			getproviders.NewRegistrySource(ctx, services, registryHTTPClient(ctx, config.RegistryProtocols), locationConfig(method)),
		), nil
	}
	switch loc := method.Location.(type) {
	case cliconfig.ProviderInstallationFilesystemMirror:
		// The path as written: tofu resolves a relative one against the
		// process's working directory, at the first install.
		return getproviders.NewFilesystemMirrorSource(ctx, string(loc)), nil

	case cliconfig.ProviderInstallationNetworkMirror:
		u, err := url.Parse(string(loc))
		if err != nil {
			return nil, fmt.Errorf("cannot parse %q as a URL for a network provider mirror: %w", string(loc), err)
		}
		if u.Scheme != "https" || u.Host == "" {
			return nil, fmt.Errorf("cannot use %q as a URL for a network provider mirror: the mirror must be at an https: URL", string(loc))
		}
		// As tofu: the registry client's timeout, though this is not a registry.
		timeout := registryHTTPClient(ctx, config.RegistryProtocols).HTTPClient.Timeout
		return getproviders.NewHTTPMirrorSource(ctx, u, services.CredentialsSource(), timeout, locationConfig(method)), nil

	case cliconfig.ProviderInstallationOCIMirror:
		return getproviders.NewOCIRegistryMirrorSource(ctx, loc.RepositoryMapping,
			func(ctx context.Context, registryDomain, repositoryName string) (getproviders.OCIRepositoryStore, error) {
				// The credentials policy is finalized only when a provider
				// is actually fetched from the mirror, as tofu has it.
				policy, err := config.OCICredentialsPolicy(ctx)
				if err != nil {
					return nil, fmt.Errorf("invalid credentials configuration for OCI registries: %w", err)
				}
				return oci.GetOCIRepositoryStore(ctx, registryDomain, repositoryName, policy)
			},
		), nil

	default:
		return nil, fmt.Errorf("unsupported provider installation method %#v", method.Location)
	}
}

// impliedProviderSource restates cmd/tofu's implicitProviderSource: the
// CLI configuration's data directories' "plugins" (~/.terraform.d/plugins,
// $XDG_DATA_HOME/opentofu/plugins) and the platform's own data directories'
// "plugins", each that exists a filesystem mirror, ahead of the registry, which
// excludes every provider those mirrors carry. tofu's first implied mirror,
// terraform.d/plugins under its launch directory, has no counterpart: a host
// serves configurations from wherever they are, not from a launch directory.
func impliedProviderSource(ctx context.Context, registry *cliconfig.RegistryProtocolsConfig, services *disco.Disco) getproviders.Source {
	var rules getproviders.MultiSource
	var found getproviders.MultiSourceMatchingPatterns
	addLocalDir := func(dir string) {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			return
		}
		mirror := getproviders.NewFilesystemMirrorSource(ctx, dir)
		// A mirror that cannot be read provides nothing, here and at install.
		if available, err := mirror.AllAvailablePackages(); err == nil {
			for addr := range available {
				found = append(found, addr)
			}
		}
		rules = append(rules, getproviders.MultiSourceSelector{Source: mirror})
	}
	if dataDirs, err := cliconfig.DataDirs(); err == nil {
		for _, dir := range dataDirs {
			addLocalDir(filepath.Join(dir, "plugins"))
		}
	}
	for _, dir := range userdirs.ForApp("Terraform", "HashiCorp", "io.terraform").DataSearchPaths("plugins") {
		addLocalDir(dir)
	}
	rules = append(rules, getproviders.MultiSourceSelector{
		Source: getproviders.NewMemoizeSource(
			getproviders.NewRegistrySource(ctx, services, registryHTTPClient(ctx, registry), getproviders.LocationConfig{
				ProviderDownloadRetries: cliconfig.ProviderDownloadRetries(),
			}),
		),
		Exclude: found,
	})
	return rules
}

// registryHTTPClient is cmd/tofu's newRegistryHTTPClient: the registry_protocols
// block's retries and timeout, OpenTofu's defaults when it has none.
func registryHTTPClient(ctx context.Context, config *cliconfig.RegistryProtocolsConfig) *retryablehttp.Client {
	if config == nil {
		config = cliconfig.BuiltinConfig.RegistryProtocols
	}
	return httpclient.NewForRegistryRequests(ctx, config.RetryCount, config.RequestTimeout)
}

// locationConfig is cmd/tofu's providerSourceLocationConfig: a method's
// download_retry_count, else TF_PROVIDER_DOWNLOAD_RETRY or its default, and
// whether the method trusts every hash it serves.
func locationConfig(method *cliconfig.ProviderInstallationMethod) getproviders.LocationConfig {
	retries, configured := 0, false
	if method.Retries != nil {
		retries, configured = method.Retries()
	}
	if !configured {
		retries = cliconfig.ProviderDownloadRetries()
	}
	return getproviders.LocationConfig{
		ProviderDownloadRetries: retries,
		TrustAllHashes:          method.Trusted != nil && method.Trusted(),
	}
}

// EnsureProvider resolves versionConstraint against the configured sources,
// installs the selected version into the cache (verifying its checksum), and
// returns the path to the runnable plugin binary along with the resolved
// version string.
//
// source is an OpenTofu provider source address ("namespace/name" or
// "hostname/namespace/name"); the default host is registry.opentofu.org. An
// empty versionConstraint means "latest".
func (i *Installer) EnsureProvider(ctx context.Context, source, versionConstraint string) (binaryPath, resolvedVersion string, err error) {
	facts, err := i.EnsureProviderFacts(ctx, source, versionConstraint)
	if err != nil {
		return "", "", err
	}
	return facts.Binary, facts.Version, nil
}

// ProviderFacts is what installing a provider established about the package
// it chose: enough for a caller to show where the plugin came from and to hand
// its binary to a scanner.
type ProviderFacts struct {
	// Provider is the provider's fully-qualified source address.
	Provider string
	// Version is the version the constraint resolved to; empty for a
	// dev_overrides directory, which is not versioned.
	Version string
	// Binary is the plugin executable, PackageDir the cached package
	// directory holding it (or a dev_overrides directory).
	Binary     string
	PackageDir string
	// Hashes are the package's checksums as a dependency lock file records
	// them (h1: over the unpacked package; zh: over the registry's archive
	// when this call downloaded it), sorted.
	Hashes []string
}

// EnsureProviderFacts is EnsureProvider reporting the installed package's
// facts rather than its binary alone.
func (i *Installer) EnsureProviderFacts(ctx context.Context, source, versionConstraint string) (ProviderFacts, error) {
	facts, _, err := i.ensure(ctx, source, versionConstraint, nil, providercache.InstallNewProvidersOnly)
	return facts, err
}

// EnsureProviderLocked is EnsureProviderFacts against a dependency lock
// file's selections, as `tofu init` installs (InitCommand.getProviders, for
// one provider): a provider prior locks already select keeps its locked
// version, and its package must match a locked checksum; upgrade selects the
// newest version the constraint admits instead (`tofu init -upgrade`). A
// provider the locks do not name resolves as EnsureProviderFacts does. A
// locked version the constraint no longer admits is an error without
// upgrade. It returns the provider's entry for the lock file init would
// write: the version, the constraint, and the prior checksums with the ones
// this install verified.
func (i *Installer) EnsureProviderLocked(ctx context.Context, source, versionConstraint string, prior *depsfile.Locks, upgrade bool) (ProviderFacts, *depsfile.ProviderLock, error) {
	mode := providercache.InstallNewProvidersOnly
	if upgrade {
		mode = providercache.InstallUpgrades
	}
	return i.ensure(ctx, source, versionConstraint, prior, mode)
}

func (i *Installer) ensure(ctx context.Context, source, versionConstraint string, prior *depsfile.Locks, mode providercache.InstallMode) (ProviderFacts, *depsfile.ProviderLock, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	addr, diags := addrs.ParseProviderSourceString(source)
	if diags.HasErrors() {
		return ProviderFacts{}, nil, fmt.Errorf("parsing provider source %q: %w", source, diags.Err())
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
		return ProviderFacts{}, nil, fmt.Errorf("parsing version constraint %q: %w", versionConstraint, err)
	}

	// Only this provider's prior selection goes in: the installer answers for
	// the requirements it is given, and so does the lock it returns.
	previous := depsfile.NewLocks()
	if prior != nil {
		if l := prior.Provider(addr); l != nil {
			previous.SetProvider(addr, l.Version(), l.VersionConstraints(), l.AllHashes())
		}
	}
	reqs := getproviders.Requirements{addr: constraints}
	locks, err := i.installer.EnsureProviderVersions(ctx, previous, reqs, mode)
	if err != nil {
		return ProviderFacts{}, nil, fmt.Errorf("installing provider %s: %w", addr, err)
	}

	lock := locks.Provider(addr)
	if lock == nil {
		return ProviderFacts{}, nil, fmt.Errorf("provider %s was not installed", addr)
	}
	version := lock.Version()

	cached := i.dir.ProviderVersion(addr, version)
	if cached == nil {
		return ProviderFacts{}, nil, fmt.Errorf("provider %s %s not found in cache after install", addr, version)
	}
	binary, err := cached.ExecutableFile()
	if err != nil {
		return ProviderFacts{}, nil, fmt.Errorf("locating provider binary for %s %s: %w", addr, version, err)
	}
	hashes := make([]string, 0, len(lock.AllHashes()))
	for _, h := range lock.AllHashes() {
		hashes = append(hashes, h.String())
	}
	sort.Strings(hashes)
	return ProviderFacts{
		Provider:   addr.String(),
		Version:    version.String(),
		Binary:     binary,
		PackageDir: cached.PackageDir,
		Hashes:     hashes,
	}, lock, nil
}
