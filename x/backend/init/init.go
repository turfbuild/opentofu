// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package init

import (
	backendInit "github.com/opentofu/opentofu/internal/backend/init"
	"github.com/opentofu/opentofu/x/backend"
	"github.com/opentofu/svchost/disco"
)

// Init initializes the backend registry, binding the service discovery object
// (and so the host credentials) that the `remote` and `cloud` protocol backends
// will use. A host should call this once at startup, before serving.
func Init(services *disco.Disco) {
	backendInit.Init(services)
}

// GetBackend returns the initialization factory for the given backend name.
// Returns nil if the backend is not found.
func GetBackend(name string) (backend.InitFn, string) {
	ensureInit()
	return backendInit.Backend(name)
}

// AvailableBackends returns the list of registered backend names.
func AvailableBackends() []string {
	return []string{
		"local", "remote", "azurerm", "consul", "cos", "gcs",
		"http", "inmem", "kubernetes", "oss", "pg", "s3", "cloud",
	}
}

// ensureInit populates the registry with anonymous service discovery when Init
// has not run yet.
//
// Backend factories are also consulted for schema-only work — rendering and
// validating a backend block — which can happen in a process that never serves
// anything, and in tests. Leaving the registry empty there would make the same
// declaration render differently depending on whether the host had started up,
// so the lookup is made unconditional. A host that needs credentialed remote
// backends still calls Init explicitly; doing so at startup, before any tool
// call, means it is the credentialed table that ends up installed.
func ensureInit() {
	if fn, _ := backendInit.Backend("local"); fn != nil {
		return
	}
	backendInit.Init(nil)
}
