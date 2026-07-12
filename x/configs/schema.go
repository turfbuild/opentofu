// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"github.com/opentofu/opentofu/internal/configs/configschema"
)

// Schema types re-exported from OpenTofu's internal configschema package.
// DecodeBodyToConfig and the objchange helpers already speak *Block at the
// boundary; these aliases let the root module name the types — to construct
// blocks from provider-supplied schemas and to call the exported Block
// methods (CoerceValue, ImpliedType, ValueMarks) directly.
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
