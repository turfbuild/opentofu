// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ProcessIgnoreChanges applies a resource's lifecycle.ignore_changes rules,
// given as paths, to the configuration value a plan starts from. It is the
// body of NodeAbstractResource.processIgnoreChanges over explicit arguments,
// for a downstream host that plans a resource instance outside a tofu.Context
// and must apply the rules at the same two points plan() does: to the
// configuration before objchange.ProposedNew and PlanResourceChange, and —
// with a nil schema — to the planned value a legacy-SDK provider returns.
//
// A null prior ignores nothing: the rules describe changes to an object that
// exists. With ignoreAll and a schema, the prior is returned with every
// computed-only attribute nulled, so that the provider is not handed a value
// it alone decides as if it had been configured; with ignoreAll and a nil
// schema the prior is returned as it is, which is what reverting a legacy
// provider's plan needs. Otherwise each path is reverted to the prior's value
// individually (processIgnoreChangesIndividual). No diagnostics are produced
// today; the signature keeps the method's.
func ProcessIgnoreChanges(prior, config cty.Value, schema *configschema.Block, ignoreChanges []cty.Path, ignoreAll bool) (cty.Value, tfdiags.Diagnostics) {
	// ignore_changes only applies when an object already exists, since we
	// can't ignore changes to a thing we've not created yet.
	if prior.IsNull() {
		return config, nil
	}

	if len(ignoreChanges) == 0 && !ignoreAll {
		return config, nil
	}

	if ignoreAll {
		// Legacy providers need up to clean up their invalid plans and ensure
		// no changes are passed though, but that also means making an invalid
		// config with computed values. In that case we just don't supply a
		// schema and return the prior val directly.
		if schema == nil {
			return prior, nil
		}

		// If we are trying to ignore all attribute changes, we must filter
		// computed attributes out from the prior state to avoid sending them
		// to the provider as if they were included in the configuration.
		ret, _ := cty.Transform(prior, func(path cty.Path, v cty.Value) (cty.Value, error) {
			attr := schema.AttributeByPath(path)
			if attr != nil && attr.Computed && !attr.Optional {
				return cty.NullVal(v.Type()), nil
			}

			return v, nil
		})

		return ret, nil
	}

	if prior.IsNull() || config.IsNull() {
		// Ignore changes doesn't apply when we're creating for the first time.
		// Proposed should never be null here, but if it is then we'll just let it be.
		return config, nil
	}

	return processIgnoreChangesIndividual(prior, config, ignoreChanges)
}

// TraversalToPath converts one ignore_changes rule, as the configuration
// decoder keeps it (configs.ManagedResource.IgnoreChanges), to the cty.Path
// the rules operate on: traversalToPath, exported for a downstream host.
func TraversalToPath(traversal hcl.Traversal) cty.Path {
	return traversalToPath(traversal)
}
