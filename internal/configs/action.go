package configs

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/opentofu/opentofu/internal/addrs"
)

// Action represents an "action" block in a configuration module: a
// provider-defined operation (Terraform 1.14+ syntax) that can be invoked
// imperatively or triggered from a resource's lifecycle via an action_trigger.
//
// OpenTofu does not yet implement actions natively (tracking
// opentofu/opentofu#3309). This is an additive, downstream-maintained extension so
// that configurations carrying the stable Terraform 1.14 action syntax parse
// cleanly rather than failing on an unsupported block type. When upstream lands
// actions, this file and its hooks (configFileSchema, the parser dispatch, the
// Module/File fields, and the lifecycle action_trigger block) collapse onto the
// upstream representation.
type Action struct {
	Type string
	Name string

	ProviderConfigRef *ProviderConfigRef
	Provider          addrs.Provider

	Count   hcl.Expression
	ForEach hcl.Expression

	// Config is the body of the nested `config` block — the action's arguments.
	// nil when the action declares no config block.
	Config hcl.Body

	DeclRange hcl.Range
	TypeRange hcl.Range
}

func (a *Action) moduleUniqueKey() string {
	return fmt.Sprintf("action.%s.%s", a.Type, a.Name)
}

// ImpliedProvider returns the provider type name an action type implies when it
// carries no explicit `provider` argument — the segment before the first
// underscore, the same rule addrs.Resource.ImpliedProvider applies to a
// resource type.
func (a *Action) ImpliedProvider() string {
	return addrs.Resource{Type: a.Type}.ImpliedProvider()
}

// ProviderConfigAddr returns the address of the provider configuration that
// should be used for this action, defaulting to the implied provider when no
// explicit "provider" argument was given. Mirror of Resource.ProviderConfigAddr.
func (a *Action) ProviderConfigAddr() addrs.LocalProviderConfig {
	if a.ProviderConfigRef == nil {
		return addrs.LocalProviderConfig{
			LocalName: a.ImpliedProvider(),
		}
	}
	return addrs.LocalProviderConfig{
		LocalName: a.ProviderConfigRef.Name,
		Alias:     a.ProviderConfigRef.Alias,
	}
}

// ActionTriggerEvent is one of the six resource lifecycle edges an
// action_trigger may fire on.
type ActionTriggerEvent string

const (
	ActionBeforeCreate  ActionTriggerEvent = "before_create"
	ActionAfterCreate   ActionTriggerEvent = "after_create"
	ActionBeforeUpdate  ActionTriggerEvent = "before_update"
	ActionAfterUpdate   ActionTriggerEvent = "after_update"
	ActionBeforeDestroy ActionTriggerEvent = "before_destroy"
	ActionAfterDestroy  ActionTriggerEvent = "after_destroy"
)

var validActionTriggerEvents = map[string]ActionTriggerEvent{
	"before_create":  ActionBeforeCreate,
	"after_create":   ActionAfterCreate,
	"before_update":  ActionBeforeUpdate,
	"after_update":   ActionAfterUpdate,
	"before_destroy": ActionBeforeDestroy,
	"after_destroy":  ActionAfterDestroy,
}

// ActionTriggerOnFailure controls what happens to the triggering operation when
// the action fails: halt (default — the operation fails and dependents are
// blocked), continue (the action's failure is downgraded to a warning and the
// operation + dependents proceed), or taint (after_create — keep the object but
// mark it for replacement next plan).
type ActionTriggerOnFailure string

const (
	ActionOnFailureHalt     ActionTriggerOnFailure = "halt"
	ActionOnFailureContinue ActionTriggerOnFailure = "continue"
	ActionOnFailureTaint    ActionTriggerOnFailure = "taint"
)

var validActionTriggerOnFailure = map[string]ActionTriggerOnFailure{
	"halt":     ActionOnFailureHalt,
	"continue": ActionOnFailureContinue,
	"taint":    ActionOnFailureTaint,
}

// ActionTrigger is a lifecycle.action_trigger block: it binds one or more
// actions to one or more lifecycle events on the containing managed resource.
type ActionTrigger struct {
	Events  []ActionTriggerEvent
	Actions []hcl.Traversal // each traversal names action.<type>.<name>

	// OnFailure is how a failure of the triggered action is handled; defaults to
	// ActionOnFailureHalt when the argument is omitted.
	OnFailure ActionTriggerOnFailure

	DeclRange hcl.Range
}

func decodeActionBlock(block *hcl.Block) (*Action, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	a := &Action{
		Type:      block.Labels[0],
		Name:      block.Labels[1],
		DeclRange: block.DefRange,
		TypeRange: block.LabelRanges[0],
	}

	if !hclsyntax.ValidIdentifier(a.Type) {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid action type name",
			Detail:   badIdentifierDetail,
			Subject:  &block.LabelRanges[0],
		})
	}
	if !hclsyntax.ValidIdentifier(a.Name) {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid action name",
			Detail:   badIdentifierDetail,
			Subject:  &block.LabelRanges[1],
		})
	}

	content, moreDiags := block.Body.Content(actionBlockSchema)
	diags = append(diags, moreDiags...)

	if attr, exists := content.Attributes["provider"]; exists {
		ref, refDiags := decodeProviderConfigRef(attr.Expr, "provider")
		diags = append(diags, refDiags...)
		a.ProviderConfigRef = ref
	}
	if attr, exists := content.Attributes["count"]; exists {
		a.Count = attr.Expr
	}
	if attr, exists := content.Attributes["for_each"]; exists {
		a.ForEach = attr.Expr
	}

	for _, b := range content.Blocks {
		if b.Type == "config" {
			a.Config = b.Body
		}
	}

	return a, diags
}

// decodeActionTriggerBlock decodes a lifecycle.action_trigger block into its
// events + action references.
func decodeActionTriggerBlock(block *hcl.Block) (*ActionTrigger, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	at := &ActionTrigger{DeclRange: block.DefRange, OnFailure: ActionOnFailureHalt}

	content, moreDiags := block.Body.Content(actionTriggerBlockSchema)
	diags = append(diags, moreDiags...)

	if attr, exists := content.Attributes["events"]; exists {
		events, evDiags := decodeTriggerEvents(attr)
		diags = append(diags, evDiags...)
		at.Events = events
	}
	if attr, exists := content.Attributes["actions"]; exists {
		actions, actDiags := decodeTriggerActions(attr)
		diags = append(diags, actDiags...)
		at.Actions = actions
	}
	if attr, exists := content.Attributes["on_failure"]; exists {
		mode, ofDiags := decodeTriggerOnFailure(attr)
		diags = append(diags, ofDiags...)
		at.OnFailure = mode
	}

	return at, diags
}

// decodeTriggerEvents decodes an action trigger's `events` list. Shared by the
// nested (lifecycle) form and the top-level address-targeted form so the two
// cannot drift.
func decodeTriggerEvents(attr *hcl.Attribute) ([]ActionTriggerEvent, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	var events []ActionTriggerEvent
	exprs, listDiags := hcl.ExprList(attr.Expr)
	diags = append(diags, listDiags...)
	for _, e := range exprs {
		kw := hcl.ExprAsKeyword(e)
		ev, ok := validActionTriggerEvents[kw]
		if !ok {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid action_trigger event",
				Detail:   "Valid events are before_create, after_create, before_update, after_update, before_destroy, and after_destroy.",
				Subject:  e.Range().Ptr(),
			})
			continue
		}
		events = append(events, ev)
	}
	return events, diags
}

// decodeTriggerActions decodes an action trigger's `actions` reference list.
func decodeTriggerActions(attr *hcl.Attribute) ([]hcl.Traversal, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	var actions []hcl.Traversal
	exprs, listDiags := hcl.ExprList(attr.Expr)
	diags = append(diags, listDiags...)
	for _, e := range exprs {
		trav, travDiags := hcl.AbsTraversalForExpr(e)
		diags = append(diags, travDiags...)
		if len(trav) > 0 {
			actions = append(actions, trav)
		}
	}
	return actions, diags
}

// decodeTriggerOnFailure decodes an action trigger's `on_failure` keyword.
func decodeTriggerOnFailure(attr *hcl.Attribute) (ActionTriggerOnFailure, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	kw := hcl.ExprAsKeyword(attr.Expr)
	mode, ok := validActionTriggerOnFailure[kw]
	if !ok {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid action_trigger on_failure",
			Detail:   "The \"on_failure\" argument requires one of the following keywords: halt, continue, or taint.",
			Subject:  attr.Expr.Range().Ptr(),
		})
		return ActionOnFailureHalt, diags
	}
	return mode, diags
}

// ActionTriggerDecl is a top-level, address-targeted action trigger block: it
// binds actions to lifecycle events on a managed resource named by address —
// including a resource inside a descendant module — without touching the
// resource's own block. Additive downstream extension, same posture as the
// action block above; there is no upstream counterpart yet.
//
//	action_trigger "commit_gate" {
//	  target     = module.network.aws_subnet.private
//	  events     = [before_create]
//	  actions    = [action.panos_commit.gate]
//	  on_failure = halt
//	}
type ActionTriggerDecl struct {
	Name string

	// Target is the managed resource the trigger attaches to, resolved
	// relative to the declaring module. Only a whole (keyless) managed
	// resource address is accepted: an instance-keyed target or a module
	// address is a decode-time error, and a data resource is refused —
	// trigger events are managed-lifecycle edges.
	Target *addrs.Target

	Events    []ActionTriggerEvent
	Actions   []hcl.Traversal // each traversal names action.<type>.<name>
	OnFailure ActionTriggerOnFailure

	// Condition is accepted by the schema and reserved: it parses, but
	// consumers refuse a trigger that carries one until action conditions
	// are implemented. Nil when absent.
	Condition hcl.Expression

	DeclRange hcl.Range
}

func (t *ActionTriggerDecl) moduleUniqueKey() string {
	return t.Name
}

// decodeActionTriggerDeclBlock decodes a top-level action_trigger block:
// the nested form's events/actions/on_failure plus the target address.
func decodeActionTriggerDeclBlock(block *hcl.Block) (*ActionTriggerDecl, hcl.Diagnostics) {
	var diags hcl.Diagnostics
	t := &ActionTriggerDecl{
		Name:      block.Labels[0],
		DeclRange: block.DefRange,
		OnFailure: ActionOnFailureHalt,
	}

	if !hclsyntax.ValidIdentifier(t.Name) {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid action_trigger name",
			Detail:   badIdentifierDetail,
			Subject:  &block.LabelRanges[0],
		})
	}

	content, moreDiags := block.Body.Content(actionTriggerDeclSchema)
	diags = append(diags, moreDiags...)

	if attr, exists := content.Attributes["target"]; exists {
		trav, travDiags := hcl.AbsTraversalForExpr(attr.Expr)
		diags = append(diags, travDiags...)
		if !travDiags.HasErrors() {
			target, targetDiags := addrs.ParseTarget(trav)
			diags = append(diags, targetDiags.ToHCL()...)
			if !targetDiags.HasErrors() {
				diags = append(diags, validateTriggerTarget(target, attr.Expr.Range())...)
				t.Target = target
			}
		}
	}

	if attr, exists := content.Attributes["events"]; exists {
		events, evDiags := decodeTriggerEvents(attr)
		diags = append(diags, evDiags...)
		t.Events = events
	}
	if attr, exists := content.Attributes["actions"]; exists {
		actions, actDiags := decodeTriggerActions(attr)
		diags = append(diags, actDiags...)
		t.Actions = actions
	}
	if attr, exists := content.Attributes["on_failure"]; exists {
		mode, ofDiags := decodeTriggerOnFailure(attr)
		diags = append(diags, ofDiags...)
		t.OnFailure = mode
	}
	if attr, exists := content.Attributes["condition"]; exists {
		t.Condition = attr.Expr
	}

	return t, diags
}

// validateTriggerTarget restricts a top-level trigger's target to a whole
// managed resource: module addresses cannot say which resource, instance keys
// are not supported yet, and data resources have no managed-lifecycle events
// to fire on.
func validateTriggerTarget(target *addrs.Target, rng hcl.Range) hcl.Diagnostics {
	var diags hcl.Diagnostics
	switch subject := target.Subject.(type) {
	case addrs.AbsResource:
		if subject.Resource.Mode != addrs.ManagedResourceMode {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid action_trigger target",
				Detail:   "The target must be a managed resource; a data resource has no lifecycle events to trigger on.",
				Subject:  rng.Ptr(),
			})
		}
	case addrs.AbsResourceInstance:
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid action_trigger target",
			Detail:   "The target must be a whole resource; an instance-keyed target is not supported. The trigger fires for every planned instance of the resource.",
			Subject:  rng.Ptr(),
		})
	default:
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid action_trigger target",
			Detail:   "The target must be a managed resource address (optionally inside a module), for example aws_instance.web or module.network.aws_subnet.private.",
			Subject:  rng.Ptr(),
		})
	}
	return diags
}

var actionBlockSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "provider"},
		{Name: "count"},
		{Name: "for_each"},
	},
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "config"},
	},
}

var actionTriggerBlockSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "events", Required: true},
		{Name: "actions", Required: true},
		{Name: "on_failure"},
	},
}

var actionTriggerDeclSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "target", Required: true},
		{Name: "events", Required: true},
		{Name: "actions", Required: true},
		{Name: "on_failure"},
		{Name: "condition"},
	},
}
