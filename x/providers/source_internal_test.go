// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package providers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
)

// TestOCIMirrorSource: an oci_mirror method builds as tofu builds it, with no
// network until a provider is fetched, and selects the providers its include
// patterns name; the rest go to direct. (A fetch through a registry is the
// opt-in suite's to witness.)
func TestOCIMirrorSource(t *testing.T) {
	file := filepath.Join(t.TempDir(), "turf.tfrc")
	body := `provider_installation {
  oci_mirror {
    repository_template = "registry.example.com/providers/${namespace}/${type}"
    include             = ["example.com/*/*"]
  }
  direct {
    exclude = ["example.com/*/*"]
  }
}
`
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TF_CLI_CONFIG_FILE", file)
	t.Setenv("TF_PLUGIN_CACHE_DIR", "")

	inst, err := NewInstaller(context.Background(), t.TempDir(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	mirrored := addrs.NewProvider("example.com", "test", "fake")
	if got := inst.source.ForDisplay(mirrored); got != "OCI registry provider mirror" {
		t.Errorf("%s is served by %q, want the OCI mirror alone", mirrored, got)
	}
	direct := addrs.NewDefaultProvider("null")
	if got := inst.source.ForDisplay(direct); got == "" || strings.Contains(got, "OCI") {
		t.Errorf("%s is served by %q, want the registry alone", direct, got)
	}
}
