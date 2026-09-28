// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package providers_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	xaddrs "github.com/opentofu/opentofu/x/addrs"
	xdepsfile "github.com/opentofu/opentofu/x/depsfile"
	xproviders "github.com/opentofu/opentofu/x/providers"
)

// fakeMirror lays out a filesystem mirror carrying example.com/test/fake at
// the given versions, as unpacked packages (no network), and points the
// installer at it.
func fakeMirror(t *testing.T, versions ...string) {
	t.Helper()
	mirror := t.TempDir()
	for _, v := range versions {
		pkg := filepath.Join(mirror, "example.com", "test", "fake", v, runtime.GOOS+"_"+runtime.GOARCH)
		if err := os.MkdirAll(pkg, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pkg, "terraform-provider-fake"), []byte("#!/bin/sh\n# "+v+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("TF_PROVIDER_MIRROR_DIR", mirror)
}

func newInstaller(t *testing.T) *xproviders.Installer {
	t.Helper()
	inst, err := xproviders.NewInstaller(context.Background(), t.TempDir(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	return inst
}

// TestEnsureProviderLocked: with no lock the newest admitted version is
// selected and locked with its checksum; a locked version holds against a
// newer one until upgrade; the lock survives the lock file; a locked version
// the constraint no longer admits, and a package matching no locked checksum,
// are refused.
func TestEnsureProviderLocked(t *testing.T) {
	ctx := context.Background()
	const source = "example.com/test/fake"
	fake := xaddrs.NewProvider("example.com", "test", "fake")

	fakeMirror(t, "1.2.3")
	facts, lock, err := newInstaller(t).EnsureProviderLocked(ctx, source, "~> 1.2", xdepsfile.NewLocks(), false)
	if err != nil {
		t.Fatal(err)
	}
	if facts.Version != "1.2.3" || lock.Version().String() != "1.2.3" || len(lock.AllHashes()) == 0 {
		t.Fatalf("selected %s, locked %s with %v", facts.Version, lock.Version(), lock.AllHashes())
	}

	// The lock file round-trips the selection.
	dir := t.TempDir()
	locks := xdepsfile.NewLocks()
	locks.SetProvider(fake, lock.Version(), lock.VersionConstraints(), lock.AllHashes())
	if err := xdepsfile.SaveLocksToDir(ctx, locks, dir); err != nil {
		t.Fatal(err)
	}
	read, err := xdepsfile.LoadLocksFromDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !read.Equal(locks) {
		t.Fatalf("read back %v, wrote %v", read.AllProviders(), locks.AllProviders())
	}
	if empty, err := xdepsfile.LoadLocksFromDir(t.TempDir()); err != nil || !empty.Empty() {
		t.Fatalf("no lock file read as %v, %v; want empty locks", empty, err)
	}

	// A newer version appears: the lock holds, upgrade moves it.
	fakeMirror(t, "1.2.3", "1.3.0")
	if facts, _, err := newInstaller(t).EnsureProviderLocked(ctx, source, "~> 1.2", read, false); err != nil || facts.Version != "1.2.3" {
		t.Errorf("locked install selected %q, %v; want 1.2.3", facts.Version, err)
	}
	if facts, _, err := newInstaller(t).EnsureProviderLocked(ctx, source, "~> 1.2", read, true); err != nil || facts.Version != "1.3.0" {
		t.Errorf("upgrade selected %q, %v; want 1.3.0", facts.Version, err)
	}
	if facts, err := newInstaller(t).EnsureProviderFacts(ctx, source, "~> 1.2"); err != nil || facts.Version != "1.3.0" {
		t.Errorf("unlocked install selected %q, %v; want 1.3.0", facts.Version, err)
	}

	// The constraint moved past the lock.
	if _, _, err := newInstaller(t).EnsureProviderLocked(ctx, source, "~> 1.3.0", read, false); err == nil ||
		!strings.Contains(err.Error(), "does not match configured version constraint") {
		t.Errorf("a lock outside the constraint installed: %v", err)
	}

	// The package is not the one locked: 1.2.3 locked with 1.3.0's checksum.
	other, _, err := newInstaller(t).EnsureProviderLocked(ctx, source, "1.3.0", xdepsfile.NewLocks(), false)
	if err != nil {
		t.Fatal(err)
	}
	if other.Hashes[0] == lock.AllHashes()[0].String() {
		t.Fatal("the two fake packages hash alike")
	}
	tampered := xdepsfile.NewLocks()
	tampered.SetProvider(fake, lock.Version(), lock.VersionConstraints(), []xdepsfile.Hash{xdepsfile.Hash(other.Hashes[0])})
	if _, _, err := newInstaller(t).EnsureProviderLocked(ctx, source, "~> 1.2", tampered, false); err == nil ||
		!strings.Contains(err.Error(), "checksum") {
		t.Errorf("a package matching no locked checksum installed: %v", err)
	}
}
