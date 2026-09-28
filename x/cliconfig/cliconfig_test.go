// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package cliconfig

import (
	"context"
	"testing"

	"github.com/opentofu/svchost"
)

// TestServiceDiscoveryResolvesEnvToken pins the wiring, not the encoding: the
// discovery object built here must actually consult the CLI configuration's
// credentials source, so a TF_TOKEN_<host> variable reaches a caller asking for
// that host's credentials. Without the credentials source, CredentialsForHost
// short-circuits to nil and every private host authenticates as nobody.
//
// The host is a fixture rather than a real one so a developer's own
// credentials.tfrc.json cannot influence the result.
func TestServiceDiscoveryResolvesEnvToken(t *testing.T) {
	// "turf-test.example.com": dots become underscores, hyphens double ones.
	t.Setenv("TF_TOKEN_turf__test_example_com", "s3cr3t")

	services, err := NewServiceDiscovery(context.Background())
	if err != nil {
		t.Fatalf("NewServiceDiscovery: %v", err)
	}

	host, err := svchost.ForComparison("turf-test.example.com")
	if err != nil {
		t.Fatalf("ForComparison: %v", err)
	}
	creds, err := services.CredentialsForHost(context.Background(), host)
	if err != nil {
		t.Fatalf("CredentialsForHost: %v", err)
	}
	if creds == nil {
		t.Fatal("no credentials resolved; the credentials source is not wired into service discovery")
	}

	// The remote backend reaches the raw token through this narrow interface.
	tokener, ok := creds.(interface{ Token() string })
	if !ok {
		t.Fatalf("credentials %T do not expose Token()", creds)
	}
	if got := tokener.Token(); got != "s3cr3t" {
		t.Errorf("Token() = %q, want %q", got, "s3cr3t")
	}
}

// TestServiceDiscoveryAlwaysUsable guards the contract that callers may ignore
// the error and still use the returned object.
func TestServiceDiscoveryAlwaysUsable(t *testing.T) {
	services, _ := NewServiceDiscovery(context.Background())
	if services == nil {
		t.Fatal("NewServiceDiscovery returned a nil *disco.Disco")
	}
}
