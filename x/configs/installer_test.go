// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/opentofu/svchost/disco"
)

// TestNewInstallerServices covers both ends of the services argument. The nil
// case is the documented fallback (anonymous discovery, public registries
// only); the non-nil case is what a host passes so a private module registry
// authenticates with the same credentials as its provider registries and
// `remote`-protocol backends.
func TestNewInstallerServices(t *testing.T) {
	for _, tc := range []struct {
		name     string
		services *disco.Disco
	}{
		{"anonymous", nil},
		{"credentialed", disco.New()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "modules")
			inst, err := NewInstaller(context.Background(), dir, tc.services)
			if err != nil {
				t.Fatalf("NewInstaller: %v", err)
			}
			if inst.Loader() == nil {
				t.Fatal("installer has no loader")
			}
		})
	}
}
