// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package encryption

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	xconfigs "github.com/opentofu/opentofu/x/configs"
)

// A statefile-shaped plaintext: DecryptState checks for terraform_version and
// that the envelope's serial and lineage match the payload's.
const plainState = `{"version":4,"terraform_version":"1.12.6","serial":3,"lineage":"a-lineage","resources":[]}`

// The usual shape: a passphrase-derived key, AES-GCM over it, state naming
// the method. The passphrase is a variable, which is what the static
// evaluator is for. Iterations at the minimum, for the test's sake.
const encryptedBlock = `
variable "passphrase" {
  type      = string
  sensitive = true
}

terraform {
  encryption {
    key_provider "pbkdf2" "k" {
      passphrase = var.passphrase
      iterations = 200000
    }
    method "aes_gcm" "m" {
      keys = key_provider.pbkdf2.k
    }
    state {
      method = method.aes_gcm.m
    }
  }
}
`

func parseModule(t *testing.T, src string, vars map[string]string) *xconfigs.Module {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	call := xconfigs.RootModuleCall(dir, "default", func(v *xconfigs.Variable) (cty.Value, hcl.Diagnostics) {
		if val, ok := vars[v.Name]; ok {
			return cty.StringVal(val), nil
		}
		return cty.NilVal, hcl.Diagnostics{&hcl.Diagnostic{Severity: hcl.DiagError, Summary: "no value for var." + v.Name}}
	})
	mod, err := xconfigs.ParseModule(dir, call)
	if err != nil {
		t.Fatal(err)
	}
	return mod
}

func mustBuild(t *testing.T, mod *xconfigs.Module, env string) Encryption {
	t.Helper()
	enc, diags := FromModule(context.Background(), mod, env)
	if diags.HasErrors() {
		t.Fatalf("FromModule: %s", diags.Error())
	}
	return enc
}

func roundTrip(t *testing.T, enc Encryption) []byte {
	t.Helper()
	ct, err := enc.State().EncryptState([]byte(plainState))
	if err != nil {
		t.Fatalf("EncryptState: %v", err)
	}
	pt, status, err := enc.State().DecryptState(ct)
	if err != nil {
		t.Fatalf("DecryptState: %v", err)
	}
	if string(pt) != plainState {
		t.Errorf("decrypted %s, want the plaintext back", pt)
	}
	if status != StatusSatisfied {
		t.Errorf("status %v, want StatusSatisfied", status)
	}
	return ct
}

// TestFromModuleBuildsTheBlock: the block is evaluated against the module's
// own static evaluator (var.passphrase), and what it writes is a payload the
// sigil check recognizes and the same configuration reads back.
func TestFromModuleBuildsTheBlock(t *testing.T) {
	mod := parseModule(t, encryptedBlock, map[string]string{"passphrase": "correct-horse-battery-staple"})
	ct := roundTrip(t, mustBuild(t, mod, ""))
	if !IsEncryptionPayload(ct) {
		t.Errorf("the written state is not an encryption payload: %s", ct)
	}
	if IsEncryptionPayload([]byte(plainState)) {
		t.Error("a plaintext statefile reads as an encryption payload")
	}
	if IsEncryptionPayload([]byte("not json")) {
		t.Error("non-JSON reads as an encryption payload")
	}

	// Another configuration — a different passphrase — cannot read it, and
	// disabled encryption passes the ciphertext through for the statefile
	// reader to refuse.
	other := mustBuild(t, parseModule(t, encryptedBlock, map[string]string{"passphrase": "a-different-passphrase"}), "")
	if _, _, err := other.State().DecryptState(ct); err == nil {
		t.Error("a different passphrase decrypted the state")
	}
	through, _, err := StateEncryptionDisabled().DecryptState(ct)
	if err != nil || string(through) != string(ct) {
		t.Errorf("disabled encryption did not pass the payload through: %v", err)
	}
}

// TestFromModuleEnvironmentOverridesTheBlock: TF_ENCRYPTION merges on top of
// the block, setting by setting, as Meta.EncryptionFromModule merges it — a
// block whose state is unencrypted is encrypted by an environment that names
// a method; and the environment alone configures a module with no block.
func TestFromModuleEnvironmentOverridesTheBlock(t *testing.T) {
	const unencryptedBlock = `
terraform {
  encryption {
    method "unencrypted" "plain" {}
    state {
      method = method.unencrypted.plain
    }
  }
}
`
	const env = `
key_provider "pbkdf2" "k" {
  passphrase = "correct-horse-battery-staple"
  iterations = 200000
}
method "aes_gcm" "m" {
  keys = key_provider.pbkdf2.k
}
state {
  method = method.aes_gcm.m
}
`
	mod := parseModule(t, unencryptedBlock, nil)
	enc, diags := FromModule(context.Background(), mod, env)
	if diags.HasErrors() {
		t.Fatalf("FromModule: %s", diags.Error())
	}
	// The block's unencrypted method is still declared, so the warning
	// upstream prints survives the merge.
	if !strings.Contains(diags.Error(), "Unencrypted method configured") {
		t.Errorf("the unencrypted method's warning is missing: %s", diags.Error())
	}
	if ct := roundTrip(t, enc); !IsEncryptionPayload(ct) {
		t.Errorf("the environment's method did not override the block's: %s", ct)
	}

	// The block alone writes plaintext.
	plainOnly := mustBuild(t, mod, "")
	if ct, err := plainOnly.State().EncryptState([]byte(plainState)); err != nil || IsEncryptionPayload(ct) {
		t.Errorf("an unencrypted block wrote %s (%v), want plaintext", ct, err)
	}

	// No block, environment only.
	envOnly := mustBuild(t, parseModule(t, `resource "null_resource" "a" {}`, nil), "  "+env+"  ")
	if ct := roundTrip(t, envOnly); !IsEncryptionPayload(ct) {
		t.Errorf("the environment alone did not encrypt: %s", ct)
	}
}

// TestFromModuleAbsentIsDisabled: no block and a blank environment is the
// pass-through encryption, so a configuration that says nothing writes
// plaintext and reads it back as satisfied.
func TestFromModuleAbsentIsDisabled(t *testing.T) {
	mod := parseModule(t, `resource "null_resource" "a" {}`, nil)
	for _, env := range []string{"", "   \n"} {
		enc := mustBuild(t, mod, env)
		ct := roundTrip(t, enc)
		if IsEncryptionPayload(ct) {
			t.Errorf("env %q: a configuration without encryption wrote a payload", env)
		}
	}
	// A nil module with no environment passes every target through unchanged.
	enc := mustBuild(t, nil, "")
	if ct, err := enc.State().EncryptState([]byte(plainState)); err != nil || string(ct) != plainState {
		t.Errorf("a nil module with no environment is not disabled: %s, %v", ct, err)
	}
}

// TestFromModuleNilModuleIsRefusedWithAnEnvironment: a host with no
// configuration directory (a backend named outright) has no root module to
// evaluate an environment's configuration in, so one is a diagnostic rather
// than a panic in New; with no environment either, it is disabled.
func TestFromModuleNilModuleIsRefusedWithAnEnvironment(t *testing.T) {
	const env = `
key_provider "pbkdf2" "k" {
  passphrase = "correct-horse-battery-staple"
  iterations = 200000
}
method "aes_gcm" "m" {
  keys = key_provider.pbkdf2.k
}
state {
  method = method.aes_gcm.m
}
`
	_, diags := FromModule(context.Background(), nil, env)
	if !diags.HasErrors() || !strings.Contains(diags.Error(), "root module") {
		t.Errorf("an environment with no module was accepted: %s", diags.Error())
	}
	if ct, err := mustBuild(t, nil, "").State().EncryptState([]byte(plainState)); err != nil || string(ct) != plainState {
		t.Errorf("a nil module with no environment is not disabled: %s, %v", ct, err)
	}
}

// TestFromModuleDiagnostics: a malformed environment and an unknown method
// are diagnostics, in OpenTofu's words.
func TestFromModuleDiagnostics(t *testing.T) {
	mod := parseModule(t, `resource "null_resource" "a" {}`, nil)
	if _, diags := FromModule(context.Background(), mod, `state { method = `); !diags.HasErrors() {
		t.Error("a malformed TF_ENCRYPTION was accepted")
	}
	_, diags := FromModule(context.Background(), mod, `state { method = method.aes_gcm.missing }`)
	if !diags.HasErrors() || !strings.Contains(diags.Error(), "undeclared encryption method") {
		t.Errorf("an unknown method was not refused in OpenTofu's words: %s", diags.Error())
	}
}

// TestFromModuleFallbackReportsMigration: a plaintext payload read under a
// configuration whose fallback is unencrypted is StatusMigration — the
// signal a host persists on, so the next write lands encrypted — and an
// enforced configuration with no fallback refuses it.
func TestFromModuleFallbackReportsMigration(t *testing.T) {
	const fallbackBlock = `
terraform {
  encryption {
    key_provider "pbkdf2" "k" {
      passphrase = "correct-horse-battery-staple"
      iterations = 200000
    }
    method "aes_gcm" "m" {
      keys = key_provider.pbkdf2.k
    }
    method "unencrypted" "plain" {}
    state {
      method = method.aes_gcm.m
      fallback {
        method = method.unencrypted.plain
      }
    }
  }
}
`
	enc := mustBuild(t, parseModule(t, fallbackBlock, nil), "")
	pt, status, err := enc.State().DecryptState([]byte(plainState))
	if err != nil || string(pt) != plainState {
		t.Fatalf("the fallback did not read plaintext: %s, %v", pt, err)
	}
	if status != StatusMigration {
		t.Errorf("status %v, want StatusMigration", status)
	}
	if ct := roundTrip(t, enc); !IsEncryptionPayload(ct) {
		t.Errorf("a write under the fallback configuration is plaintext: %s", ct)
	}

	enforced := mustBuild(t, parseModule(t, strings.Replace(encryptedBlock, "method = method.aes_gcm.m", "method = method.aes_gcm.m\n      enforced = true", 1),
		map[string]string{"passphrase": "correct-horse-battery-staple"}), "")
	if _, _, err := enforced.State().DecryptState([]byte(plainState)); err == nil || !strings.Contains(err.Error(), "unencrypted payload") {
		t.Errorf("an enforced configuration read plaintext: %v", err)
	}
}
