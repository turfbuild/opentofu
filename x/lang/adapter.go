// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package lang

import (
	"context"
	"strconv"
	"strings"

	otflang "github.com/opentofu/opentofu/internal/lang"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// scopeData adapts a *Scope to OpenTofu's lang.Data interface so we can
// reuse OpenTofu's expression evaluator (lang.Scope.EvalExpr) directly.
//
// The adapter is uniformly permissive: every Get* method returns
// cty.DynamicVal for references that aren't satisfied by the scope.
// Unknown values propagate through cty semantics, which is the right
// shape for walker contexts and re-evaluation of walker-emitted token
// Config.
//
// Strict semantics (fail fast on unresolved refs) are layered on top via
// ValidateExpression / ValidateConfig — called by user-facing tools
// before evaluation so a missing reference surfaces as a clear "you
// referenced X but X isn't available" diagnostic rather than silently
// propagating __cty_unknown__ down to a provider.
type scopeData struct {
	scope *Scope
}

var _ otflang.Data = (*scopeData)(nil)

// StaticValidateReferences intentionally returns nil. Validation lives in
// ValidateExpression / ValidateConfig at the caller layer; the adapter's
// job is pure value resolution.
func (d *scopeData) StaticValidateReferences(_ context.Context, _ []*addrs.Reference, _ addrs.Referenceable, _ addrs.Referenceable) tfdiags.Diagnostics {
	return nil
}

func (d *scopeData) GetCountAttr(_ context.Context, addr addrs.CountAttr, _ tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	v, _ := resolveRef(d.scope, addr)
	return v, nil
}

func (d *scopeData) GetForEachAttr(_ context.Context, addr addrs.ForEachAttr, _ tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	v, _ := resolveRef(d.scope, addr)
	return v, nil
}

func (d *scopeData) GetResource(_ context.Context, addr addrs.Resource, _ tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	v, _ := resolveRef(d.scope, addr)
	return v, nil
}

func (d *scopeData) GetLocalValue(_ context.Context, addr addrs.LocalValue, _ tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	v, _ := resolveRef(d.scope, addr)
	return v, nil
}

func (d *scopeData) GetModule(_ context.Context, addr addrs.ModuleCall, _ tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	v, _ := resolveRef(d.scope, addr)
	return v, nil
}

func (d *scopeData) GetPathAttr(_ context.Context, addr addrs.PathAttr, _ tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	v, _ := resolveRef(d.scope, addr)
	return v, nil
}

func (d *scopeData) GetTerraformAttr(_ context.Context, addr addrs.TerraformAttr, _ tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	v, _ := resolveRef(d.scope, addr)
	return v, nil
}

func (d *scopeData) GetInputVariable(_ context.Context, addr addrs.InputVariable, _ tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	v, _ := resolveRef(d.scope, addr)
	return v, nil
}

func (d *scopeData) GetOutput(_ context.Context, addr addrs.OutputValue, _ tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	v, _ := resolveRef(d.scope, addr)
	return v, nil
}

func (d *scopeData) GetCheckBlock(_ context.Context, addr addrs.Check, _ tfdiags.SourceRange) (cty.Value, tfdiags.Diagnostics) {
	v, _ := resolveRef(d.scope, addr)
	return v, nil
}

// resolveRef looks up the value backing an addrs.Referenceable in the scope.
// Returns (value, true) when the reference resolves, (DynamicVal, false)
// otherwise. The boolean is the strict-validation signal; callers in
// permissive mode (adapter) discard it, while ValidateExpression uses it
// to drive diagnostics.
func resolveRef(scope *Scope, subj addrs.Referenceable) (cty.Value, bool) {
	switch s := subj.(type) {
	case addrs.InputVariable:
		if v, ok := scope.Variables[s.Name]; ok {
			return v, true
		}
	case addrs.LocalValue:
		if v, ok := scope.Locals[s.Name]; ok {
			return v, true
		}
	case addrs.Resource:
		var store map[string]cty.Value
		switch s.Mode {
		case addrs.DataResourceMode:
			store = scope.DataSources
		case addrs.EphemeralResourceMode:
			store = scope.Ephemerals
		default:
			store = scope.Resources
		}
		if v, ok := lookupResourceLike(store, s.Type+"."+s.Name); ok {
			return v, true
		}
	case addrs.ModuleCall:
		// Matching Terraform/OpenTofu semantics, a module call resolves only to
		// its declared outputs (registered via SetModuleOutput) — resources
		// inside the module are not reachable through the module namespace.
		if v, ok := scope.Modules[s.Name]; ok {
			return v, true
		}
	case addrs.OutputValue:
		if v, ok := scope.Outputs[s.Name]; ok {
			return v, true
		}
	case addrs.PathAttr:
		switch s.Name {
		case "module":
			if scope.Path.Module != "" {
				return cty.StringVal(scope.Path.Module), true
			}
		case "root":
			if scope.Path.Root != "" {
				return cty.StringVal(scope.Path.Root), true
			}
		case "cwd":
			if scope.Path.Cwd != "" {
				return cty.StringVal(scope.Path.Cwd), true
			}
		}
	case addrs.TerraformAttr:
		if s.Name == "workspace" {
			return cty.StringVal("default"), true
		}
	case addrs.CountAttr:
		if scope.Count != nil && s.Name == "index" {
			return cty.NumberIntVal(int64(*scope.Count)), true
		}
	case addrs.ForEachAttr:
		if scope.Each != nil {
			switch s.Name {
			case "key":
				return scope.Each.Key, true
			case "value":
				return scope.Each.Value, true
			}
		}
	}
	return cty.DynamicVal, false
}

// lookupResourceLike scans a flat addr->value store for entries matching
// either the bare "<type>.<name>" address or instance-keyed forms
// "<type>.<name>[<key>]". Returns the bare value if present, else an
// aggregated cty value (tuple for int keys, object for string keys). The
// boolean reports whether anything matched.
func lookupResourceLike(store map[string]cty.Value, prefix string) (cty.Value, bool) {
	var bare cty.Value
	intInstances := map[int]cty.Value{}
	strInstances := map[string]cty.Value{}
	maxIdx := -1

	for addr, val := range store {
		if addr == prefix {
			bare = val
			continue
		}
		if !strings.HasPrefix(addr, prefix+"[") || !strings.HasSuffix(addr, "]") {
			continue
		}
		inner := addr[len(prefix)+1 : len(addr)-1]
		if strings.HasPrefix(inner, `"`) && strings.HasSuffix(inner, `"`) {
			strInstances[strings.Trim(inner, `"`)] = val
		} else if i, err := strconv.Atoi(inner); err == nil {
			intInstances[i] = val
			if i > maxIdx {
				maxIdx = i
			}
		}
	}

	switch {
	case bare != cty.NilVal:
		return bare, true
	case len(intInstances) > 0:
		tuple := make([]cty.Value, maxIdx+1)
		for i := range tuple {
			if v, ok := intInstances[i]; ok {
				tuple[i] = v
			} else {
				tuple[i] = cty.DynamicVal
			}
		}
		return cty.TupleVal(tuple), true
	case len(strInstances) > 0:
		// Heterogeneous values are common (mix of real cty values + DynamicVal),
		// so use ObjectVal rather than MapVal which would panic on type mismatch.
		return cty.ObjectVal(strInstances), true
	default:
		return cty.NilVal, false
	}
}
