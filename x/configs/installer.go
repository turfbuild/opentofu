// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"context"
	"fmt"
	"os"
	"runtime/trace"

	version "github.com/hashicorp/go-version"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configload"
	"github.com/opentofu/opentofu/internal/getmodules"
	"github.com/opentofu/opentofu/internal/initwd"
	"github.com/opentofu/opentofu/internal/registry"
	"github.com/opentofu/svchost/disco"
)

// traceModuleHooks logs each module download/install into the execution trace
// (runtime/trace) so `go tool trace` shows which modules a registry install
// resolved and fetched, and where the time went. Download fires only for
// modules pulled from a remote source (the registry/git/http hit); Install
// fires for every module including local ones. Embeds the no-op impl so new
// hook methods added upstream keep compiling. Inert when tracing is off.
type traceModuleHooks struct {
	initwd.ModuleInstallHooksImpl
	ctx context.Context
}

func (h traceModuleHooks) Download(moduleAddr, packageAddr string, _ *version.Version) {
	trace.Log(h.ctx, "module.download", moduleAddr+" <- "+packageAddr)
}

func (h traceModuleHooks) Install(moduleAddr string, _ *version.Version, _ string) {
	trace.Log(h.ctx, "module.install", moduleAddr)
}

// Installer wraps OpenTofu's module installer to download and install
// modules referenced by module blocks in HCL configuration.
type Installer struct {
	modsDir   string
	loader    *configload.Loader
	installer *initwd.ModuleInstaller
}

// NewInstaller creates a module installer that downloads modules into modulesDir.
// The modulesDir will be created if it does not exist.
//
// services carries the host credentials used to reach a module registry; pass
// the same object the rest of the host uses (see x/cliconfig.NewServiceDiscovery)
// so a private registry authenticates. It is the same credential set that
// reaches provider registries and the `remote`-protocol backends — one host,
// one token. A nil services falls back to anonymous discovery, which reaches
// public registries only.
func NewInstaller(ctx context.Context, modulesDir string, services *disco.Disco) (*Installer, error) {
	if err := os.MkdirAll(modulesDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create modules directory: %w", err)
	}
	loader, err := configload.NewLoader(&configload.Config{
		ModulesDir: modulesDir,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create config loader: %w", err)
	}

	// A nil services is what registry.NewClient itself substitutes an anonymous
	// disco for, so it is passed through rather than filled in here.
	regClient := registry.NewClient(ctx, services, nil)

	// Module *packages* (git, https, S3, OCI) are fetched by go-getter, which
	// takes its credentials from the environment and from the source address
	// rather than from service discovery. Only the registry protocol —
	// resolving a registry address to a package address — authenticates through
	// services. A fetcher environment would additionally supply an OCI
	// credential store; turf does not configure one yet.
	fetcher := getmodules.NewPackageFetcher(ctx, nil)

	inst := initwd.NewModuleInstaller(modulesDir, loader, regClient, fetcher)

	return &Installer{
		modsDir:   modulesDir,
		loader:    loader,
		installer: inst,
	}, nil
}

// InstallModules downloads and installs all modules referenced by the configuration
// in rootDir. This is equivalent to `tofu init` for module installation.
func (inst *Installer) InstallModules(ctx context.Context, rootDir string) error {
	defer trace.StartRegion(ctx, "registry.ModuleInstall").End()
	_, diags := inst.installer.InstallModules(
		ctx,
		rootDir,
		"",    // testsDir — not needed
		false, // upgrade — don't force re-download
		false, // installErrsOnly — report all errors
		traceModuleHooks{ctx: ctx},
		configs.StaticModuleCall{},
	)
	if diags.HasErrors() {
		// Return the first error
		for _, diag := range diags {
			if diag.Severity().String() == "Error" {
				return fmt.Errorf("module installation failed: %s", diag.Description().Detail)
			}
		}
		return fmt.Errorf("module installation failed")
	}

	// Refresh the loader's module manifest so LoadConfig can find installed modules
	if err := inst.loader.RefreshModules(); err != nil {
		return fmt.Errorf("failed to refresh module manifest: %w", err)
	}

	return nil
}

// Loader returns a Loader that can load the full config tree after module installation.
func (inst *Installer) Loader() *Loader {
	return &Loader{loader: inst.loader}
}
