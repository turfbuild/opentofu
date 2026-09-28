// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package providers_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	xproviders "github.com/opentofu/opentofu/x/providers"
)

// TestEnsureProviderFacts installs a provider from a filesystem mirror (an
// unpacked package, so no network) and checks the facts it reports: the
// resolved version, the binary inside the cached package directory, and an h1
// checksum of the package as a lock file would record it.
func TestEnsureProviderFacts(t *testing.T) {
	mirror := t.TempDir()
	pkg := filepath.Join(mirror, "example.com", "test", "fake", "1.2.3", runtime.GOOS+"_"+runtime.GOARCH)
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "terraform-provider-fake"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TF_PROVIDER_MIRROR_DIR", mirror)

	cache := t.TempDir()
	inst, err := xproviders.NewInstaller(context.Background(), cache, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := inst.EnsureProviderFacts(context.Background(), "example.com/test/fake", "~> 1.2")
	if err != nil {
		t.Fatal(err)
	}
	if facts.Provider != "example.com/test/fake" || facts.Version != "1.2.3" {
		t.Errorf("facts = %+v, want example.com/test/fake 1.2.3", facts)
	}
	if !strings.HasPrefix(facts.PackageDir, cache) || filepath.Dir(facts.Binary) != facts.PackageDir {
		t.Errorf("binary %q in package %q, want both under the cache %q", facts.Binary, facts.PackageDir, cache)
	}
	if filepath.Base(facts.Binary) != "terraform-provider-fake" {
		t.Errorf("binary = %q", facts.Binary)
	}
	h1 := false
	for _, h := range facts.Hashes {
		h1 = h1 || strings.HasPrefix(h, "h1:")
	}
	if !h1 {
		t.Errorf("hashes = %v, want an h1: checksum of the package", facts.Hashes)
	}

	// The binary-only form agrees.
	binary, version, err := inst.EnsureProvider(context.Background(), "example.com/test/fake", "1.2.3")
	if err != nil || binary != facts.Binary || version != facts.Version {
		t.Errorf("EnsureProvider = %q %q %v, want %q %q", binary, version, err, facts.Binary, facts.Version)
	}
}
