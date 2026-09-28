// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package cliconfig

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
	// An empty configuration of the test's own, so neither the developer's
	// files nor their TF_PLUGIN_CACHE_DIR can put it in error.
	empty := filepath.Join(t.TempDir(), "empty.tfrc")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TF_CLI_CONFIG_FILE", empty)
	t.Setenv("TF_PLUGIN_CACHE_DIR", "")
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

// TestServiceDiscoveryKeepsWhatLoaded: a CLI configuration in error (here a
// plugin_cache_dir that does not exist) is used as it loaded, as tofu uses
// it. Its credentials block and TF_TOKEN_<host> both still authenticate, and
// the error comes back as advice.
func TestServiceDiscoveryKeepsWhatLoaded(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "turf.tfrc")
	body := `plugin_cache_dir = "` + filepath.ToSlash(filepath.Join(dir, "missing")) + `"
credentials "turf-file.example.com" {
  token = "from-file"
}
`
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TF_CLI_CONFIG_FILE", file)
	t.Setenv("TF_PLUGIN_CACHE_DIR", "")
	t.Setenv("TF_TOKEN_turf__env_example_com", "from-env")

	services, err := NewServiceDiscovery(context.Background())
	if err == nil || !strings.Contains(err.Error(), "plugin cache dir") {
		t.Errorf("err = %v, want the configuration's error as advice", err)
	}
	for host, want := range map[string]string{"turf-file.example.com": "from-file", "turf-env.example.com": "from-env"} {
		h, err := svchost.ForComparison(host)
		if err != nil {
			t.Fatal(err)
		}
		creds, err := services.CredentialsForHost(context.Background(), h)
		if err != nil {
			t.Fatalf("CredentialsForHost(%s): %v", host, err)
		}
		tokener, ok := creds.(interface{ Token() string })
		if !ok || tokener.Token() != want {
			t.Errorf("%s: credentials %#v, want token %q", host, creds, want)
		}
	}

	// A file the environment names that does not exist is only a warning to
	// LoadConfig; it still comes back, for the host to report.
	t.Setenv("TF_CLI_CONFIG_FILE", filepath.Join(dir, "missing.tfrc"))
	if _, err := NewServiceDiscovery(context.Background()); err == nil || !strings.Contains(err.Error(), "missing.tfrc") {
		t.Errorf("err = %v, want the missing file named", err)
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
