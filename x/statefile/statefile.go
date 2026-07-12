// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

// Package statefile is a stable boundary over OpenTofu's internal
// states/statefile package. It exposes the File wrapper that jsonstate/jsonplan
// marshalling expects around an in-memory state, so a consumer can render prior_state
// without importing OpenTofu internals directly.
package statefile

import (
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/statefile"
)

// File pairs an in-memory state with the lineage/serial metadata the state-file
// marshallers read.
type File = statefile.File

// New wraps an in-memory state as a *File for marshalling. lineage/serial are
// carried through verbatim; pass "" / 0 when the caller has no persisted
// lineage (jsonstate marshalling reads only File.State).
func New(state *states.State, lineage string, serial uint64) *File {
	return statefile.New(state, lineage, serial)
}
