// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

// Package state re-exports OpenTofu's internal state types through a stable API boundary.
// This prevents direct imports of OpenTofu internals from consuming modules.
// Addresses — including the provider configuration a resource is recorded
// under — live in the sibling x/addrs package.
package state

import (
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

// Core state types.
type State = states.State
type Module = states.Module
type Resource = states.Resource
type ResourceInstanceObjectSrc = states.ResourceInstanceObjectSrc

// Object status.
type ObjectStatus = states.ObjectStatus

const (
	ObjectReady   = states.ObjectReady
	ObjectTainted = states.ObjectTainted
	ObjectPlanned = states.ObjectPlanned
)

// Deposed keys for create-before-destroy.
type DeposedKey = states.DeposedKey

const NotDeposed = states.NotDeposed

var NewDeposedKey = states.NewDeposedKey

// State constructors.
var NewState = states.NewState

// State manager interfaces.
type Full = statemgr.Full
type Locker = statemgr.Locker
type LockInfo = statemgr.LockInfo
type LockError = statemgr.LockError

// Migrator is the optional manager extension that reads and writes a whole
// statefile, header included — lineage, serial and the recorded version —
// rather than only the state it carries. Implemented by the filesystem
// manager, remote.State (the whole remote-backend family) and cloud.State.
type Migrator = statemgr.Migrator

// SnapshotTerraformVersion reports the OpenTofu version recorded in the state
// snapshot the given manager most recently read or persisted, and whether it is
// known at all.
//
// Unknown (false) means either that the manager does not implement
// PersistentMeta — statemgr.NewFullFake does not — or that it has no persisted
// snapshot yet, which is the state of a workspace nothing has written to. It is
// deliberately distinct from an empty version string so a caller can tell
// "nothing has written this state" from "something wrote it without saying
// what".
//
// Note this is not the version the calling program links against: the point of
// the call is to report who wrote the bytes, which is the same question
// `tofu show -json` answers with its terraform_version field. A manager's
// exported statefile.File cannot answer it — statefile.New stamps the running
// version — which is why the manager's own snapshot metadata is the source.
func SnapshotTerraformVersion(m Full) (string, bool) {
	pm, ok := m.(statemgr.PersistentMeta)
	if !ok {
		return "", false
	}
	v := pm.StateSnapshotMeta().TerraformVersion
	if v == nil {
		return "", false
	}
	return v.String(), true
}

// State manager helpers.
var NewLockInfo = statemgr.NewLockInfo

// NewLineage mints a new state lineage: the identity a line of state
// snapshots shares from its first write on.
var NewLineage = statemgr.NewLineage

// NewFullFake returns an in-memory statemgr.Full backed by a transient store.
// Intended for tests that need a Manager without standing up a real backend.
var NewFullFake = statemgr.NewFullFake
