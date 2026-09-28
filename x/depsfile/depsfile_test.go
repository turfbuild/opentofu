// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package depsfile

import (
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
)

// TestLoadLocksFromBytes pins the captured-content reader: the selection a lock
// file records, empty content as empty locks, and malformed content as an
// error.
func TestLoadLocksFromBytes(t *testing.T) {
	src := []byte(`
provider "registry.opentofu.org/hashicorp/time" {
  version     = "0.13.1"
  constraints = "~> 0.13"
  hashes = [
    "h1:ZT5ppCNIModqk3iOkVt5my8V5f7hO0ENMUt5bPyiIIY=",
  ]
}
`)
	locks, err := LoadLocksFromBytes(src, "pinned.lock.hcl")
	if err != nil {
		t.Fatal(err)
	}
	lock := locks.Provider(addrs.MustParseProviderSourceString("hashicorp/time"))
	if lock == nil || lock.Version().String() != "0.13.1" {
		t.Fatalf("lock = %#v", lock)
	}

	empty, err := LoadLocksFromBytes(nil, "pinned.lock.hcl")
	if err != nil || !empty.Empty() {
		t.Fatalf("empty content: %v, %#v", err, empty)
	}

	if _, err := LoadLocksFromBytes([]byte(`provider "x" {`), "pinned.lock.hcl"); err == nil || !strings.Contains(err.Error(), "Unclosed configuration block") {
		t.Fatalf("malformed content: %v", err)
	}
}
