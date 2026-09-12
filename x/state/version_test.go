// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

// TestSnapshotTerraformVersion pins the two answers a caller has to be able to
// tell apart: a version that a real snapshot records, and "nobody wrote this".
//
// The filesystem manager is driven against a snapshot recording an *older*
// version, because that is the only fixture where reporting the running version
// instead — which is what every manager did before the snapshot metadata
// carried it — looks different from reporting the truth.
func TestSnapshotTerraformVersion(t *testing.T) {
	const priorVersion = "1.5.7"

	path := filepath.Join(t.TempDir(), "terraform.tfstate")
	if err := os.WriteFile(path, []byte(`{
	"version": 4,
	"terraform_version": "`+priorVersion+`",
	"serial": 7,
	"lineage": "5d3f5f8e-0d3f-4f22-9a7a-6f3f2b1c0a11",
	"outputs": {},
	"resources": []
}
`), 0o600); err != nil {
		t.Fatalf("seeding statefile: %s", err)
	}

	mgr := statemgr.NewFilesystem(path, encryption.StateEncryptionDisabled())
	if err := mgr.RefreshState(t.Context()); err != nil {
		t.Fatalf("refreshing: %s", err)
	}
	got, ok := SnapshotTerraformVersion(mgr)
	if !ok || got != priorVersion {
		t.Errorf("SnapshotTerraformVersion() = (%q, %t); want (%q, true)", got, ok, priorVersion)
	}

	// A manager that cannot answer must say so rather than inventing a version:
	// the caller's fallback ("what the next write will stamp") is only correct
	// when it knows it is guessing.
	if got, ok := SnapshotTerraformVersion(statemgr.NewFullFake(nil, NewState())); ok {
		t.Errorf("SnapshotTerraformVersion(fake) = (%q, true); want unknown", got)
	}
}
