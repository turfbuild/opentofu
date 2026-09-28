// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/zclconf/go-cty/cty"
)

// StaticModuleVariables answers one variable declaration with the value the
// caller has for it, or a diagnostic explaining why it has none. The static
// evaluator calls it lazily, once per variable actually referenced from a
// statically-evaluated position, which is what lets a host resolve values it
// can only obtain after parsing has begun.
type StaticModuleVariables = configs.StaticModuleVariables

// RootModuleCall builds the root-module static call that statically-evaluated
// positions resolve against: module source and version, the backend block,
// provider for_each, const variables, and the path.root / terraform.workspace
// symbols. Every entry point in this package requires one, because a zero
// StaticModuleCall carries a nil vars func that the evaluator invokes without
// a guard.
//
// This mirrors Meta.rootModuleCall (internal/command/meta_config.go), which a
// non-CLI host cannot reach. Where the CLI prompts on a terminal for a missing
// required variable, a host supplies whatever interaction it has through vars.
//
// A nil vars func is replaced with one that returns a diagnostic naming the
// variable, so a caller with no variable values at all gets an error at the
// reference rather than a panic. An empty workspace resolves to "default",
// matching both OpenTofu's default state name and x/lang's evaluation scope,
// so an unplumbed caller cannot make terraform.workspace an empty string.
func RootModuleCall(rootPath, workspace string, vars StaticModuleVariables) StaticModuleCall {
	if vars == nil {
		vars = noStaticVariables
	}
	if workspace == "" {
		workspace = "default"
	}
	return configs.NewStaticModuleCall(addrs.RootModule, hcl.Range{}, vars, rootPath, workspace)
}

func noStaticVariables(v *configs.Variable) (cty.Value, hcl.Diagnostics) {
	return cty.NilVal, hcl.Diagnostics{&hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "No value for input variable",
		Detail: fmt.Sprintf(
			"The value of var.%s is needed here, but this configuration was loaded without any variable values.",
			v.Name),
		Subject: v.DeclRange.Ptr(),
	}}
}
