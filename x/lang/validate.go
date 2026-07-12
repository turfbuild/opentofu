// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package lang

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/addrs"
	otflang "github.com/opentofu/opentofu/internal/lang"
)

// ValidateExpression checks whether every reference in expr is satisfied
// by the given scope. Returns nil when every ref resolves; otherwise
// returns an error listing the unresolved references.
//
// This is the strict pre-flight that user-facing tools should run before
// calling EvalExpression / EvalConfig. The evaluator is permissive
// (missing refs → cty.DynamicVal), so without an eager validator a typo
// like ${var.regin} would silently produce __cty_unknown__ in the
// evaluated config and surface downstream as a confusing provider error.
//
// Walker contexts intentionally skip this check — they expect unresolved
// references to propagate as unknowns through the cty type system.
//
// resolveLocals uses this as a dependency-ordering signal: a local whose
// expression has at least one unresolved ref defers to the next pass.
func ValidateExpression(expr hcl.Expression, scope *Scope) error {
	refs, refDiags := otflang.ReferencesInExpr(addrs.ParseRef, expr)
	if refDiags.HasErrors() {
		return fmt.Errorf("parse references: %s", refDiags.Err())
	}
	var missing []string
	seen := map[string]bool{}
	add := func(label string) {
		if !seen[label] {
			seen[label] = true
			missing = append(missing, label)
		}
	}
	moduleOutputMissing := false
	for _, ref := range refs {
		// A module-output ref must resolve at OUTPUT granularity, not just at
		// the module call: `module.<name>` exposes declared outputs and nothing
		// else (Terraform/OpenTofu semantics), so a ref that names a missing
		// output — including a resource address inside the module, which parses
		// as an output ref (`module.p.random_pet` for module.p.random_pet.x.id)
		// — must fail here rather than leak the literal `${…}` downstream.
		if out, modName, ok := moduleOutputRef(ref.Subject); ok {
			modVal, found := resolveRef(scope, addrs.ModuleCall{Name: modName})
			switch {
			case found && (!modVal.Type().IsObjectType() || modVal.Type().HasAttribute(out)):
				// Call resolves and the output is present — or the module is
				// keyed (tuple/map value), which we don't disprove here.
			case !found:
				// Call itself absent from this scope. Still module-shaped, so
				// carry the declared-outputs guidance: the fix is the same
				// (declare an output / fetch it out-of-band), whether the call
				// is missing or the output is.
				add("module." + modName)
				moduleOutputMissing = true
			default:
				add("module." + modName + "." + out) // resolves, but no such output
				moduleOutputMissing = true
			}
			continue
		}
		subj := normalizeRefSubject(ref.Subject)
		if _, ok := resolveRef(scope, subj); ok {
			continue
		}
		add(formatRefSubject(subj))
	}
	if len(missing) == 0 {
		return nil
	}
	msg := "unresolved references: " + strings.Join(missing, ", ")
	if moduleOutputMissing {
		msg += " (a module call exposes only its declared outputs, module.<name>.<output> — " +
			"resources inside the module are not addressable; declare an output on the module " +
			"or fetch the value with module_outputs and pass it as a literal. Module outputs " +
			"are not persisted to state, so a committed-phase output cannot be referenced from " +
			"a later phase; reference it within the same phase instead)"
	}
	return fmt.Errorf("%s", msg)
}

// moduleOutputRef reports whether subj is a single-level module-output
// reference (module.<name>.<output>) and returns the output and module names.
// Used by ValidateExpression to resolve such references at output granularity
// rather than collapsing them to the containing module call.
func moduleOutputRef(subj addrs.Referenceable) (output, module string, ok bool) {
	if s, isOut := subj.(addrs.ModuleCallInstanceOutput); isOut {
		return s.Name, s.Call.Call.Name, true
	}
	return "", "", false
}

// ValidateConfig recursively walks a config map and validates every
// expression string it contains. Returns the first error encountered, or
// nil when every expression's refs are satisfied.
func ValidateConfig(config map[string]any, scope *Scope) error {
	for key, val := range config {
		if err := validateValue(val, scope); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
	}
	return nil
}

func validateValue(val any, scope *Scope) error {
	switch v := val.(type) {
	case string:
		if !ContainsInterpolation(v) {
			return nil
		}
		expr, diags := hclsyntax.ParseTemplate([]byte(v), "", hcl.InitialPos)
		if diags.HasErrors() {
			return fmt.Errorf("parse: %s", diags.Error())
		}
		return ValidateExpression(expr, scope)
	case map[string]any:
		for k, mv := range v {
			if err := validateValue(mv, scope); err != nil {
				return fmt.Errorf("%s: %w", k, err)
			}
		}
	case []any:
		for i, sv := range v {
			if err := validateValue(sv, scope); err != nil {
				return fmt.Errorf("[%d]: %w", i, err)
			}
		}
	}
	return nil
}

// normalizeRefSubject collapses instance-keyed subjects to their containing
// container, matching OpenTofu's putValueBySubject pattern. The walker
// stores values by container address (no instance key), so the lookup must
// match that shape.
func normalizeRefSubject(subj addrs.Referenceable) addrs.Referenceable {
	switch s := subj.(type) {
	case addrs.ResourceInstance:
		return s.ContainingResource()
	case addrs.ModuleCallInstance:
		return s.Call
	case addrs.ModuleCallInstanceOutput:
		return s.Call.Call
	}
	return subj
}

// formatRefSubject renders a Referenceable as the canonical HCL-style
// string (var.X, local.Y, module.Z, data.T.N, etc.) for error messages.
func formatRefSubject(subj addrs.Referenceable) string {
	switch s := subj.(type) {
	case addrs.InputVariable:
		return "var." + s.Name
	case addrs.LocalValue:
		return "local." + s.Name
	case addrs.ModuleCall:
		return "module." + s.Name
	case addrs.Resource:
		if s.Mode == addrs.DataResourceMode {
			return fmt.Sprintf("data.%s.%s", s.Type, s.Name)
		}
		return fmt.Sprintf("%s.%s", s.Type, s.Name)
	case addrs.OutputValue:
		return "output." + s.Name
	case addrs.PathAttr:
		return "path." + s.Name
	case addrs.TerraformAttr:
		return "terraform." + s.Name
	case addrs.CountAttr:
		return "count." + s.Name
	case addrs.ForEachAttr:
		return "each." + s.Name
	}
	return subj.String()
}

// ExpressionLocalRefs returns the names of every local.X reference inside
// expr. Used by resolveLocals to decide whether to defer a local's
// evaluation until other locals in the same module finish resolving.
func ExpressionLocalRefs(expr hcl.Expression) ([]string, error) {
	refs, refDiags := otflang.ReferencesInExpr(addrs.ParseRef, expr)
	if refDiags.HasErrors() {
		return nil, fmt.Errorf("parse references: %s", refDiags.Err())
	}
	var out []string
	for _, ref := range refs {
		if lv, ok := ref.Subject.(addrs.LocalValue); ok {
			out = append(out, lv.Name)
		}
	}
	return out, nil
}
