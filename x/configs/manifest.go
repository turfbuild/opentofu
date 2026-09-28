// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"github.com/opentofu/opentofu/internal/modsdir"
)

// ModuleManifest is the module installer's record of each module call it
// resolved -- modules.json under the modules directory, keyed by module path
// ("a.b") -- which the loader reads back to find every module's directory.
type ModuleManifest = modsdir.Manifest

// ModuleRecord is one module's entry in a ModuleManifest: its key, the source
// address it was installed from, its version, and its directory.
type ModuleRecord = modsdir.Record

// ReadModuleManifest reads the manifest under modulesDir. A directory without
// one reads as an empty manifest, as the installer treats it.
func ReadModuleManifest(modulesDir string) (ModuleManifest, error) {
	return modsdir.ReadManifestSnapshotForDir(modulesDir)
}

// WriteModuleManifest writes m as the manifest under modulesDir, replacing
// any there.
func WriteModuleManifest(modulesDir string, m ModuleManifest) error {
	return m.WriteSnapshotToDir(modulesDir)
}
