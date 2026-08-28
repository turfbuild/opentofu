// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"github.com/opentofu/opentofu/internal/addrs"
)

// Reference is a parsed expression reference: the subject being referenced,
// the source range it was written at, and any remaining traversal past the
// subject. It is the unit OpenTofu's evaluator consumes when building an
// hcl.EvalContext, so a consumer assembling one needs to name this type.
type Reference = addrs.Reference

// The remaining reference subjects. Referenceable and the module/variable
// subjects are declared in addrs.go; these are the ones an implementation of
// lang.Data must handle but which had no alias, which is what made that
// interface unimplementable from outside this facade.
type (
	CountAttr        = addrs.CountAttr
	ForEachAttr      = addrs.ForEachAttr
	PathAttr         = addrs.PathAttr
	TerraformAttr    = addrs.TerraformAttr
	Check            = addrs.Check
	ProviderFunction = addrs.ProviderFunction
)
