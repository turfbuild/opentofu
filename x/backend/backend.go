// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package backend

import (
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/svchost/disco"
)

// Backend is the main interface for backend implementations.
// Re-exported from OpenTofu's internal backend package.
type Backend = backend.Backend

// Enhanced is a backend that can also run operations and contribute service
// discovery aliases (the `remote` and `cloud` protocol backends).
// Re-exported from OpenTofu's internal backend package.
type Enhanced = backend.Enhanced

// HostAlias is a service-discovery alias contributed by an enhanced backend.
// Re-exported from OpenTofu's internal backend package.
type HostAlias = backend.HostAlias

// InitFn is a function that initializes a backend with the given configuration.
// Re-exported from OpenTofu's internal backend package.
type InitFn = backend.InitFn

// DefaultStateName is the name of the default state in backends that support workspaces.
const DefaultStateName = backend.DefaultStateName

// Sentinel errors returned by backends that address exactly one remote
// workspace (`remote` configured with `workspaces { name }`): every state name
// other than DefaultStateName is unsupported, and DefaultStateName itself is
// unsupported in the `workspaces { prefix }` form.
var (
	ErrDefaultWorkspaceNotSupported = backend.ErrDefaultWorkspaceNotSupported
	ErrWorkspacesNotSupported       = backend.ErrWorkspacesNotSupported
)

// ReconcileRemoteVersion mirrors the tofu CLI's Meta.remoteVersionCheck for
// backends that report a remote OpenTofu version (`remote`, `cloud`).
//
// Those backends carry a coarse fall-back check in StateMgr that demands an
// exact string match between the local version and the workspace's configured
// version. The CLI never lets that check fire: it runs the real compatibility
// check first — which is a no-op for a workspace whose execution mode is
// `local`, where the remote version is meaningless — and then suppresses the
// fall-back. Callers that use a backend purely for state storage must do the
// same before touching StateMgr.
//
// Returns nil for backends that do not report a remote version.
//
// The interface is restated here rather than imported from internal/command,
// which would drag the whole CLI into every consumer of this facade.
func ReconcileRemoteVersion(b Backend, workspace string) error {
	type backendWithRemoteVersion interface {
		IgnoreVersionConflict()
		VerifyWorkspaceTerraformVersion(workspace string) tfdiags.Diagnostics
	}
	remote, ok := b.(backendWithRemoteVersion)
	if !ok {
		return nil
	}
	if diags := remote.VerifyWorkspaceTerraformVersion(workspace); diags.HasErrors() {
		return diags.Err()
	}
	remote.IgnoreVersionConflict()
	return nil
}

// SetupServiceDiscoveryAliases mirrors the tofu CLI's
// Meta.setupEnhancedBackendAliases: an enhanced backend may map a generic
// hostname onto the host it is actually configured for, and service discovery
// has to learn that mapping. Returns nil for backends that are not enhanced.
func SetupServiceDiscoveryAliases(b Backend, services *disco.Disco) error {
	enhanced, ok := b.(Enhanced)
	if !ok || services == nil {
		return nil
	}
	aliases, err := enhanced.ServiceDiscoveryAliases()
	if err != nil {
		return err
	}
	for _, alias := range aliases {
		services.Alias(alias.From, alias.To)
	}
	return nil
}
