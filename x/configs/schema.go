// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"github.com/hashicorp/hcl/v2"

	"github.com/opentofu/opentofu/internal/configs/configschema"
)

// Schema types re-exported from OpenTofu's internal configschema package.
// The objchange helpers already speak *Block at the boundary; these aliases
// let a consumer name the types — to construct blocks from provider-supplied
// schemas and to call the exported Block methods (DecoderSpec, CoerceValue,
// ImpliedType, ValueMarks) directly.
type (
	Block       = configschema.Block
	Attribute   = configschema.Attribute
	NestedBlock = configschema.NestedBlock
	Object      = configschema.Object
	NestingMode = configschema.NestingMode
)

// Nesting modes for NestedBlock and Object (re-export from configschema).
const (
	NestingSingle = configschema.NestingSingle
	NestingGroup  = configschema.NestingGroup
	NestingList   = configschema.NestingList
	NestingSet    = configschema.NestingSet
	NestingMap    = configschema.NestingMap
)

// StaticValidateTraversal checks that a relative traversal — an
// ignore_changes rule, as ManagedResource.IgnoreChanges keeps it — names
// something the block's schema has, in the words OpenTofu's validate walk uses
// (Block.StaticValidateTraversal: an unsupported attribute, an indexed set
// block, a list block without a numeric index). The diagnostics come back as
// an error, so callers never import tfdiags.
func StaticValidateTraversal(b *Block, traversal hcl.Traversal) error {
	diags := b.StaticValidateTraversal(traversal)
	if diags.HasErrors() {
		return diags.Err()
	}
	return nil
}
