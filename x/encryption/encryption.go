// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

// Package encryption re-exports OpenTofu's state encryption: the
// configuration a root module declares in its `terraform { encryption {} }`
// block (and TF_ENCRYPTION merges into), the Encryption built from it, and
// the per-target overlays a backend or a statefile reader is handed.
package encryption

import (
	"context"
	"strings"

	"github.com/hashicorp/hcl/v2"

	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/encryption/config"
	xconfigs "github.com/opentofu/opentofu/x/configs"
)

// Encryption is a built encryption configuration: State() is the overlay a
// backend reads and writes the statefile through, Plan() the one for plan
// files, and RemoteState(name) the one a terraform_remote_state read uses.
// A target the configuration does not name gets a pass-through overlay.
type Encryption = encryption.Encryption

// StateEncryption represents the encryption configuration for state files.
// Re-exported from OpenTofu's internal encryption package.
type StateEncryption = encryption.StateEncryption

// EncryptionStatus is what a decrypt reports about the payload it read:
// StatusSatisfied when the primary method read it, StatusMigration when a
// fallback did — an unencrypted payload under an `unencrypted` fallback
// included — so that the next write moves the payload to the primary method.
type EncryptionStatus = encryption.EncryptionStatus

const (
	StatusUnknown   = encryption.StatusUnknown
	StatusSatisfied = encryption.StatusSatisfied
	StatusMigration = encryption.StatusMigration
)

// EnvName is the environment variable OpenTofu reads an encryption
// configuration from, in the same HCL (or JSON) as the block, without the
// `terraform { encryption {} }` wrapper.
const EnvName = "TF_ENCRYPTION"

// StateEncryptionDisabled returns a disabled state encryption configuration.
func StateEncryptionDisabled() StateEncryption {
	return encryption.StateEncryptionDisabled()
}

// IsEncryptionPayload reports whether data is an encrypted payload: a JSON
// document carrying the encryption sigil. Anything else — a plaintext
// statefile, bytes that are not JSON — is false.
func IsEncryptionPayload(data []byte) bool {
	ok, err := encryption.IsEncryptionPayload(data)
	return err == nil && ok
}

// FromModule builds the encryption a root module asks for: its encryption
// block merged with envConfig — the text of TF_ENCRYPTION, which overrides
// the block setting by setting — evaluated through the module's own static
// evaluator, so var.*, local.* and terraform.workspace resolve as they do in
// its backend block, with the default registry of key providers and methods.
//
// This mirrors Meta.EncryptionFromModule (internal/command/meta_encryption.go),
// which a non-CLI host cannot reach, with the environment read left to the
// host: it passes the value it holds, from whatever environment it reads its
// variables from, rather than this process's. A blank envConfig is unset.
//
// No block and no envConfig is disabled: every target passes through
// unchanged. The configuration is evaluated in
// the scope of a root module, so a nil module (a host with no configuration
// directory) with an envConfig is an error diagnostic, as is a module loaded
// without static evaluation — never the panic New would raise.
//
// Key providers run here, as New runs them: a passphrase derivation, a KMS
// call. Build once, and only when the state is about to be read.
func FromModule(ctx context.Context, mod *xconfigs.Module, envConfig string) (Encryption, hcl.Diagnostics) {
	var (
		cfg   *config.EncryptionConfig
		diags hcl.Diagnostics
	)
	if mod != nil {
		cfg = mod.Encryption
	}
	if env := strings.TrimSpace(envConfig); env != "" {
		envCfg, envDiags := config.LoadConfigFromString(EnvName, env)
		diags = append(diags, envDiags...)
		if envDiags.HasErrors() {
			return nil, diags
		}
		cfg = cfg.Merge(envCfg)
	}
	if cfg == nil {
		return encryption.Disabled(), diags
	}
	if mod == nil || mod.StaticEvaluator == nil {
		return nil, append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Encryption configuration cannot be evaluated",
			Detail:   "An encryption configuration is evaluated in the scope of a root module, and this one was given none to evaluate in.",
			Subject:  cfg.DeclRange.Ptr(),
		})
	}
	enc, encDiags := encryption.New(ctx, encryption.DefaultRegistry, cfg, mod.StaticEvaluator)
	diags = append(diags, encDiags...)
	if encDiags.HasErrors() {
		return nil, diags
	}
	return enc, diags
}
