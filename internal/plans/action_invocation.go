// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plans

// ActionInvocationInstanceSrc describes a single planned invocation of a
// provider action (the Terraform 1.14 actions feature). It is the plan-level
// counterpart of the configuration-level Action/ActionTrigger blocks parsed in
// package configs.
//
// OpenTofu does not yet implement actions natively (tracking
// opentofu/opentofu#3309). This is an additive, downstream-maintained extension
// so that a plan can carry action invocations alongside its resource and output
// changes rather than forcing callers to keep a parallel side-table. It is
// in-memory only: there is intentionally no planproto serialization yet, because
// no caller persists a plan file with invocations. When upstream lands actions,
// this type collapses onto the upstream representation (which will likely model
// the config payload as a cty DynamicValue and the provider as an
// addrs.AbsProviderConfig); the field set here is deliberately the minimum a
// downstream apply needs.
type ActionInvocationInstanceSrc struct {
	// Addr is the action's address: action.<type>.<name>.
	Addr string

	// Type is the provider action type — the InvokeAction RPC target.
	Type string

	// ProviderConfigRef is the provider configuration reference as authored
	// ("name" or "name.alias"); "" when derived from the action type prefix.
	ProviderConfigRef string

	// Resolved provider coordinates, recorded at plan time so apply needs no
	// re-resolution. (Upstream's AbsProviderConfig does not track a version;
	// downstream does, so these are carried explicitly.)
	//
	// ProviderHostname is the registry host, empty for the default registry.
	// It is part of the address, not decoration: a namespace and type do not
	// identify a provider on their own, and an invocation that drops the host
	// resolves at apply to a different provider than the one the plan chose —
	// which then has no configured instance behind it.
	ProviderHostname  string
	ProviderNamespace string
	ProviderName      string
	ProviderVersion   string
	ProviderAlias     string

	// TriggeringResourceAddr is the resource whose lifecycle edge triggered this
	// invocation; "" for a standalone (untriggered) invocation.
	TriggeringResourceAddr string

	// TriggerEvent is the lifecycle edge that fired the invocation — one of
	// before_create … after_destroy; "" when standalone.
	TriggerEvent string

	// Config carries the action's config arguments: literal values where the
	// authored expression is constant, and "${...}" strings where it references
	// other objects (re-evaluated against state + outputs at apply).
	//
	// When ConfigFrozen is true the map instead holds fully resolved values,
	// captured at plan time, and apply uses them verbatim.
	Config map[string]any

	// ConfigFrozen marks a config that was resolved at plan time (destroy-event
	// triggers, whose references cannot be re-evaluated at apply without
	// ordering cycles — the Terraform 1.16 "frozen at plan" rule). Apply must
	// not re-evaluate a frozen config: a resolved literal that happens to
	// contain "${" must not be re-parsed as a template.
	ConfigFrozen bool

	// Refs are the resource addresses the config references, derived at plan time.
	// They drive execution ordering (the invocation waits on the objects it reads).
	// Always nil when ConfigFrozen: a frozen config reads nothing at apply.
	Refs []string

	// OnFailure is how a failure of this action is handled — one of "halt"
	// (default), "continue", or "taint" — from the triggering action_trigger's
	// on_failure argument. "" is treated as "halt".
	OnFailure string
}
