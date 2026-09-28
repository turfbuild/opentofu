// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/tofu"
)

// ProcessIgnoreChanges applies a resource's lifecycle.ignore_changes rules to
// the configuration a plan starts from, as OpenTofu's plan does before
// objchange.ProposedNew and PlanResourceChange — on the marked values, so a
// reverted path takes the prior's marks too — and, with a nil schema, to the
// planned value a legacy-SDK provider returns. A null prior ignores nothing.
// With ignoreAll the prior is returned, its computed-only attributes nulled
// when a schema is given.
//
// This wraps OpenTofu's tofu.ProcessIgnoreChanges, the body of
// NodeAbstractResource.processIgnoreChanges over explicit arguments. Its
// diagnostics come back as an error, so callers never import tfdiags.
func ProcessIgnoreChanges(prior, config cty.Value, schema *configschema.Block, ignoreChanges []cty.Path, ignoreAll bool) (cty.Value, error) {
	v, diags := tofu.ProcessIgnoreChanges(prior, config, schema, ignoreChanges, ignoreAll)
	if diags.HasErrors() {
		return cty.NilVal, diags.Err()
	}
	return v, nil
}

// TraversalToPath converts one ignore_changes rule, as configs.ManagedResource
// keeps it (IgnoreChanges), to the path ProcessIgnoreChanges operates on.
//
// This wraps OpenTofu's tofu.TraversalToPath.
func TraversalToPath(traversal hcl.Traversal) cty.Path {
	return tofu.TraversalToPath(traversal)
}
