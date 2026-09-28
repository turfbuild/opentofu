// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package encryption

import (
	"github.com/opentofu/opentofu/internal/encryption"
)

// StateEncryption represents the encryption configuration for state files.
// Re-exported from OpenTofu's internal encryption package.
type StateEncryption = encryption.StateEncryption

// StateEncryptionDisabled returns a disabled state encryption configuration.
func StateEncryptionDisabled() StateEncryption {
	return encryption.StateEncryptionDisabled()
}
