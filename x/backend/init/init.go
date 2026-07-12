// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package init

import (
	backendInit "github.com/opentofu/opentofu/internal/backend/init"
	"github.com/opentofu/opentofu/x/backend"
	"github.com/opentofu/svchost/disco"
)

// Init initializes the backend registry. Must be called before using backends.
func Init(services *disco.Disco) {
	backendInit.Init(services)
}

// GetBackend returns the initialization factory for the given backend name.
// Returns nil if the backend is not found.
func GetBackend(name string) (backend.InitFn, string) {
	return backendInit.Backend(name)
}

// AvailableBackends returns the list of registered backend names.
func AvailableBackends() []string {
	return []string{
		"local", "remote", "azurerm", "consul", "cos", "gcs",
		"http", "inmem", "kubernetes", "oss", "pg", "s3", "cloud",
	}
}
