// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package backend

import (
	"github.com/opentofu/opentofu/internal/backend"
)

// Backend is the main interface for backend implementations.
// Re-exported from OpenTofu's internal backend package.
type Backend = backend.Backend

// InitFn is a function that initializes a backend with the given configuration.
// Re-exported from OpenTofu's internal backend package.
type InitFn = backend.InitFn

// DefaultStateName is the name of the default state in backends that support workspaces.
const DefaultStateName = backend.DefaultStateName
