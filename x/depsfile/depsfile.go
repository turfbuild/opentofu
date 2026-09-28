// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

// Package depsfile is the dependency lock file (.terraform.lock.hcl): the
// provider versions and checksums `tofu init` selects and records beside a
// configuration, and every later run honours.
package depsfile

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/opentofu/opentofu/internal/depsfile"
	"github.com/opentofu/opentofu/internal/getproviders"
)

// LockFilePath is the lock file's name in a configuration's root module
// directory.
const LockFilePath = depsfile.LockFilePath

// Locks is a lock file's content: one ProviderLock per locked provider.
type Locks = depsfile.Locks

// ProviderLock is one provider's selection: the version, the constraints it
// was selected under, and the checksums of its packages.
type ProviderLock = depsfile.ProviderLock

// Hash is a provider package's checksum as a lock file records it: "h1:…"
// over the unpacked package, "zh:…" over the registry's archive.
type Hash = getproviders.Hash

// NewLocks is an empty set of locks.
func NewLocks() *Locks {
	return depsfile.NewLocks()
}

// ProviderIsLockable reports whether a provider is recorded in a lock file:
// built-in providers are not.
var ProviderIsLockable = depsfile.ProviderIsLockable

// LoadLocksFromDir reads the lock file in a configuration's root module
// directory. A directory with no lock file has empty locks, not an error.
// Mirrors Meta.lockedDependencies, without the dev-override annotation a
// CLI configuration adds.
func LoadLocksFromDir(dir string) (*Locks, error) {
	filename := filepath.Join(dir, LockFilePath)
	if _, err := os.Stat(filename); errors.Is(err, fs.ErrNotExist) {
		return NewLocks(), nil
	}
	locks, diags := depsfile.LoadLocksFromFile(filename)
	if diags.HasErrors() {
		return nil, diags.Err()
	}
	return locks, nil
}

// SaveLocksToDir writes locks as the lock file in a configuration's root
// module directory, replacing it atomically.
func SaveLocksToDir(ctx context.Context, locks *Locks, dir string) error {
	return depsfile.SaveLocksToFile(ctx, locks, filepath.Join(dir, LockFilePath)).Err()
}
