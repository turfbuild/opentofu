// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package lang

import (
	"strings"
	"testing"

	"github.com/zclconf/go-cty/cty"
)

// TestValidateConfig_ModuleOutputGranularity: a module call exposes only its
// declared outputs, so a module-output reference must be validated at OUTPUT
// granularity — a reference to an output the call does not declare (or to a
// resource inside the module, which parses as an output ref) is rejected
// rather than leaking the literal `${module.p.out}` into provider config. A
// module with no registered outputs at all is unresolved at call level.
func TestValidateConfig_ModuleOutputGranularity(t *testing.T) {
	t.Run("CommittedPhaseModuleUnresolvedAtCallLevel", func(t *testing.T) {
		s := NewScope()
		// module.p exists in state via its resources, but carries NO registered
		// outputs — the committed-phase shape. State-seeded resource entries do
		// not make the call resolvable.
		s.SetResource("module.p.random_pet.x", cty.ObjectVal(map[string]cty.Value{
			"id": cty.StringVal("first-turtle"),
		}))

		err := ValidateConfig(map[string]any{"keepers": "${module.p.name}"}, s)
		if err == nil {
			t.Fatal("expected a validation error for a committed-phase module output, got nil")
		}
		if !strings.Contains(err.Error(), "module.p") {
			t.Fatalf("error should name the unresolved module call; got: %v", err)
		}
	})

	t.Run("RegisteredOutputsMissingNameRejectedAtOutputGranularity", func(t *testing.T) {
		s := NewScope()
		s.SetModuleOutput("p", cty.ObjectVal(map[string]cty.Value{
			"other": cty.StringVal("present"),
		}))

		err := ValidateConfig(map[string]any{"keepers": "${module.p.name}"}, s)
		if err == nil {
			t.Fatal("expected a validation error for an undeclared module output, got nil")
		}
		if !strings.Contains(err.Error(), "module.p.name") {
			t.Fatalf("error should name the missing output module.p.name; got: %v", err)
		}
		if !strings.Contains(err.Error(), "declared outputs") {
			t.Fatalf("error should state the declared-outputs contract; got: %v", err)
		}
	})

	t.Run("InPhaseOutputPresentPasses", func(t *testing.T) {
		s := NewScope()
		// In-phase walked module: outputs threaded through scope.
		s.SetModuleOutput("p", cty.ObjectVal(map[string]cty.Value{
			"name": cty.StringVal("first-turtle"),
		}))

		if err := ValidateConfig(map[string]any{"keepers": "${module.p.name}"}, s); err != nil {
			t.Fatalf("in-phase module output should validate; got: %v", err)
		}
	})

	t.Run("InPhaseOutputUnknownStillPasses", func(t *testing.T) {
		s := NewScope()
		// Output exists but is computed/unknown — a real slot, value pending.
		// Validation must pass (it resolves to __cty_unknown__, tier-2).
		s.SetModuleOutput("p", cty.ObjectVal(map[string]cty.Value{
			"name": cty.UnknownVal(cty.String),
		}))

		if err := ValidateConfig(map[string]any{"keepers": "${module.p.name}"}, s); err != nil {
			t.Fatalf("unknown-but-present module output should validate; got: %v", err)
		}
	})

	t.Run("MixedTemplateValidatesEachInterpolation", func(t *testing.T) {
		s := NewScope()
		s.SetResource("random_pet.a", cty.ObjectVal(map[string]cty.Value{
			"id": cty.StringVal("able-mongoose"),
		}))
		s.SetResource("random_pet.b", cty.ObjectVal(map[string]cty.Value{
			"id": cty.StringVal("brave-otter"),
		}))

		// A mixed template (interp-literal-interp) must validate as a template,
		// checking each interpolation — not get mis-parsed as one whole-string
		// expression. Both refs resolve, so this passes.
		if err := ValidateConfig(map[string]any{"combo": "${random_pet.a.id}-${random_pet.b.id}"}, s); err != nil {
			t.Fatalf("mixed template with resolvable refs should validate; got: %v", err)
		}
	})

	t.Run("MixedTemplateReportsUnresolvedInterpolation", func(t *testing.T) {
		s := NewScope()
		s.SetResource("random_pet.a", cty.ObjectVal(map[string]cty.Value{
			"id": cty.StringVal("able-mongoose"),
		}))
		// random_pet.b is absent — the second interpolation must surface as an
		// unresolved reference rather than passing silently.
		err := ValidateConfig(map[string]any{"combo": "${random_pet.a.id}-${random_pet.b.id}"}, s)
		if err == nil {
			t.Fatal("expected an unresolved-reference error for the missing ref, got nil")
		}
		if !strings.Contains(err.Error(), "random_pet.b") {
			t.Fatalf("error should name the unresolved ref random_pet.b; got: %v", err)
		}
	})

	t.Run("EscapedSequenceIsLiteralNotAnExpression", func(t *testing.T) {
		s := NewScope()
		// "$${x}" is an escaped literal "${x}", not an interpolation — it must
		// validate (no references to resolve).
		if err := ValidateConfig(map[string]any{"literal": "$${x}"}, s); err != nil {
			t.Fatalf("escaped sequence should validate as a literal; got: %v", err)
		}
	})

	t.Run("AbsentModuleCallCarriesDeclaredOutputsGuidance", func(t *testing.T) {
		s := NewScope()
		// module.p is absent entirely — neither outputs nor state resources.
		// The diagnostic names the call and, because the reference is
		// module-shaped, carries the declared-outputs guidance (the remedy is
		// the same whether the call or the output is missing).
		err := ValidateConfig(map[string]any{"keepers": "${module.p.name}"}, s)
		if err == nil {
			t.Fatal("expected a validation error for an absent module call, got nil")
		}
		if !strings.Contains(err.Error(), "module.p") {
			t.Fatalf("error should name the absent module call; got: %v", err)
		}
		if !strings.Contains(err.Error(), "declared outputs") {
			t.Fatalf("module-shaped miss should carry the declared-outputs guidance; got: %v", err)
		}
	})
}
