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
	"strconv"
	"strings"
	"testing"

	xaddrs "github.com/opentofu/opentofu/x/addrs"
	xproviders "github.com/opentofu/opentofu/x/providers"
)

// cliConfig writes body as the one CLI configuration file the process reads
// (TF_CLI_CONFIG_FILE, which also skips the developer's ~/.terraform.d), and
// returns its path. The environment's own contributions to the configuration
// are cleared too: a developer's TF_PLUGIN_CACHE_DIR naming a directory that
// does not exist would put it in error.
func cliConfig(t *testing.T, body string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "turf.tfrc")
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TF_CLI_CONFIG_FILE", file)
	t.Setenv("TF_PLUGIN_CACHE_DIR", "")
	t.Setenv("TF_PLUGIN_CACHE_MAY_BREAK_DEPENDENCY_LOCK_FILE", "")
	return file
}

// mirrorConfig declares a filesystem mirror final for example.com's
// providers, the shape a host's operator writes for a private mirror.
func mirrorConfig(t *testing.T, mirror string) {
	t.Helper()
	cliConfig(t, `provider_installation {
  filesystem_mirror {
    path    = `+strconv.Quote(mirror)+`
    include = ["example.com/*/*"]
  }
  direct {
    exclude = ["example.com/*/*"]
  }
}
`)
}

// mirrorPackage lays out example.com/test/<typ> at version in a filesystem
// mirror, as an unpacked package for this platform (no network).
func mirrorPackage(t *testing.T, mirror, typ, version string) {
	t.Helper()
	pkg := filepath.Join(mirror, "example.com", "test", typ, version, runtime.GOOS+"_"+runtime.GOARCH)
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "terraform-provider-"+typ), []byte("#!/bin/sh\n# "+version+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestEnsureProviderFacts installs a provider from a filesystem mirror the CLI
// configuration declares and checks the facts it reports: the resolved
// version, the binary inside the cached package directory, and an h1 checksum
// of the package as a lock file would record it.
func TestEnsureProviderFacts(t *testing.T) {
	mirror := t.TempDir()
	mirrorPackage(t, mirror, "fake", "1.2.3")
	mirrorConfig(t, mirror)

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

// TestImpliedMirrors: with no provider_installation block, the user's
// ~/.terraform.d/plugins and $XDG_DATA_HOME/opentofu/plugins are mirrors, and
// the registry is not asked for what they carry (it would be, for
// example.com, and fail). HOME and the XDG directories point at temporary
// ones, so a developer's own plugin directories cannot take part.
func TestImpliedMirrors(t *testing.T) {
	home, data := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_DATA_DIRS", t.TempDir())
	cliConfig(t, "")
	mirrorPackage(t, filepath.Join(home, ".terraform.d", "plugins"), "fake", "1.2.3")
	mirrorPackage(t, filepath.Join(data, "opentofu", "plugins"), "other", "0.1.0")

	inst, err := xproviders.NewInstaller(context.Background(), t.TempDir(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	for source, want := range map[string]string{"example.com/test/fake": "1.2.3", "example.com/test/other": "0.1.0"} {
		facts, err := inst.EnsureProviderFacts(context.Background(), source, "")
		if err != nil || facts.Version != want {
			t.Errorf("%s: installed %q, %v; want %s from the implied mirror", source, facts.Version, err, want)
		}
	}
}

// TestConfigurationInError: a CLI configuration in error is refused, where
// tofu reports it and carries on with what loaded. What failed may be the
// provider_installation block itself: a method that does not decode leaves
// the others (here, direct alone, which would fetch from the registry what the
// block meant to mirror), and a file that does not parse leaves nothing, so
// the implied sources would install everything.
func TestConfigurationInError(t *testing.T) {
	for name, body := range map[string]string{
		"a file that does not parse": `credentials "example.com" {
  token = "x"

provider_installation {
  filesystem_mirror {
    path    = "/srv/providers"
    include = ["example.com/*/*"]
  }
  direct {
    exclude = ["example.com/*/*"]
  }
}
`,
		"an error with no block at all": `plugin_cache_dir = "/nonexistent/turf-test/plugin-cache"
`,
		"a mirror without its path": `provider_installation {
  filesystem_mirror {
    include = ["example.com/*/*"]
  }
  direct {
    exclude = ["example.com/*/*"]
  }
}
`,
		"a network mirror over http": `provider_installation {
  network_mirror {
    url = "http://mirror.example.com/providers/"
  }
}
`,
		"dev_overrides after a method": `provider_installation {
  direct {}
  dev_overrides {
    "example.com/test/fake" = "/opt/fake"
  }
}
`,
	} {
		t.Run(name, func(t *testing.T) {
			cliConfig(t, body)
			if _, err := xproviders.NewInstaller(context.Background(), t.TempDir(), "", nil); err == nil {
				t.Error("NewInstaller accepted it")
			}
		})
	}

	// The file the environment names is missing: LoadConfig warns and goes
	// on without it, to the implied sources.
	cliConfig(t, "")
	t.Setenv("TF_CLI_CONFIG_FILE", filepath.Join(t.TempDir(), "missing.tfrc"))
	if _, err := xproviders.NewInstaller(context.Background(), t.TempDir(), "", nil); err == nil {
		t.Error("NewInstaller accepted a TF_CLI_CONFIG_FILE naming no file")
	}
}

// TestDevOverrides: a dev_overrides entry is the launch-time substitution
// tofu makes. Installing the provider is unaffected (tofu init installs and
// locks the published version, and only warns); DevOverride reports the
// override directory's plugin, unversioned, for the host to launch instead.
func TestDevOverrides(t *testing.T) {
	ctx := context.Background()
	mirror, override := t.TempDir(), t.TempDir()
	mirrorPackage(t, mirror, "fake", "1.2.3")
	if err := os.WriteFile(filepath.Join(override, "terraform-provider-fake"), []byte("#!/bin/sh\n# dev\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cliConfig(t, `provider_installation {
  dev_overrides {
    "example.com/test/fake" = `+strconv.Quote(override)+`
    "example.com/test/gone" = `+strconv.Quote(filepath.Join(override, "nothing-here"))+`
  }
  filesystem_mirror {
    path    = `+strconv.Quote(mirror)+`
    include = ["example.com/*/*"]
  }
  direct {
    exclude = ["example.com/*/*"]
  }
}
`)
	inst, err := xproviders.NewInstaller(ctx, t.TempDir(), "", nil)
	if err != nil {
		t.Fatal(err)
	}

	got := inst.DevOverrides()
	want := []xaddrs.Provider{xaddrs.NewProvider("example.com", "test", "fake"), xaddrs.NewProvider("example.com", "test", "gone")}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("DevOverrides() = %v, want %v", got, want)
	}

	facts, ok, err := inst.DevOverride("example.com/test/fake")
	if err != nil || !ok {
		t.Fatalf("DevOverride = %v, %v", ok, err)
	}
	if facts.Binary != filepath.Join(override, "terraform-provider-fake") || facts.Version != "" || facts.PackageDir != override || len(facts.Hashes) != 0 {
		t.Errorf("DevOverride facts = %+v, want the override's plugin, unversioned", facts)
	}
	if _, ok, err := inst.DevOverride("example.com/test/other"); ok || err != nil {
		t.Errorf("DevOverride of a provider not overridden = %v, %v", ok, err)
	}
	if _, ok, err := inst.DevOverride("example.com/test/gone"); !ok || err == nil {
		t.Errorf("DevOverride of a directory without a plugin = %v, %v; want an error", ok, err)
	}

	installed, lock, err := inst.EnsureProviderLocked(ctx, "example.com/test/fake", "~> 1.2", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if installed.Version != "1.2.3" || installed.Binary == facts.Binary || lock == nil || len(lock.AllHashes()) == 0 {
		t.Errorf("installed %+v, lock %v; want the mirror's 1.2.3, locked", installed, lock)
	}
}
