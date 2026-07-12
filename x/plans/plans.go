// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

// Package plans is a stable boundary over OpenTofu's internal plans package.
// It re-exports the canonical plan-time types (Plan, Changes, ResourceInstanceChangeSrc,
// Action, ResourceInstanceChangeActionReason, DynamicValue, …) so consumers can
// model plan-time information using TF-faithful structures without importing
// OpenTofu internals directly.
//
// Companion file render.go provides display helpers that map
// between the rune-typed Action enum and the lowercase JSON vocabulary that
// `tofu show -json` emits, plus the display-symbol map used
// by prompts.
package plans

import (
	"github.com/opentofu/opentofu/internal/plans"
)

// Top-level plan types.
type (
	Plan        = plans.Plan
	Changes     = plans.Changes
	ChangesSync = plans.ChangesSync
	Mode        = plans.Mode
)

// Resource-instance change types.
type (
	ResourceInstanceChange    = plans.ResourceInstanceChange
	ResourceInstanceChangeSrc = plans.ResourceInstanceChangeSrc
	Change                    = plans.Change
	ChangeSrc                 = plans.ChangeSrc
)

// Output change types.
type (
	OutputChange    = plans.OutputChange
	OutputChangeSrc = plans.OutputChangeSrc
)

// Action-invocation type (Terraform 1.14 actions, downstream extension). Carried
// inside Changes.ActionInvocations.
type ActionInvocationInstanceSrc = plans.ActionInvocationInstanceSrc

// Import descriptors (carried inside a Change/ChangeSrc).
type (
	Importing    = plans.Importing
	ImportingSrc = plans.ImportingSrc
)

// Action vocabulary and reason metadata.
type (
	Action                             = plans.Action
	ResourceInstanceChangeActionReason = plans.ResourceInstanceChangeActionReason
)

// DynamicValue is the msgpack-encoded carrier used inside ChangeSrc.Before/After.
// Construct via NewDynamicValue; decode via the value's Decode(ty) method.
type DynamicValue = plans.DynamicValue

// Plan modes.
const (
	NormalMode      = plans.NormalMode
	DestroyMode     = plans.DestroyMode
	RefreshOnlyMode = plans.RefreshOnlyMode
)

// Action constants.
const (
	NoOp             = plans.NoOp
	Create           = plans.Create
	Read             = plans.Read
	Update           = plans.Update
	Delete           = plans.Delete
	DeleteThenCreate = plans.DeleteThenCreate
	CreateThenDelete = plans.CreateThenDelete
	Forget           = plans.Forget
)

// ResourceInstanceChangeActionReason constants (full set).
const (
	ResourceInstanceChangeNoReason                = plans.ResourceInstanceChangeNoReason
	ResourceInstanceReplaceBecauseTainted         = plans.ResourceInstanceReplaceBecauseTainted
	ResourceInstanceReplaceByRequest              = plans.ResourceInstanceReplaceByRequest
	ResourceInstanceReplaceByTriggers             = plans.ResourceInstanceReplaceByTriggers
	ResourceInstanceReplaceBecauseCannotUpdate    = plans.ResourceInstanceReplaceBecauseCannotUpdate
	ResourceInstanceDeleteBecauseNoResourceConfig = plans.ResourceInstanceDeleteBecauseNoResourceConfig
	ResourceInstanceDeleteBecauseWrongRepetition  = plans.ResourceInstanceDeleteBecauseWrongRepetition
	ResourceInstanceDeleteBecauseCountIndex       = plans.ResourceInstanceDeleteBecauseCountIndex
	ResourceInstanceDeleteBecauseEachKey          = plans.ResourceInstanceDeleteBecauseEachKey
	ResourceInstanceDeleteBecauseEnabledFalse     = plans.ResourceInstanceDeleteBecauseEnabledFalse
	ResourceInstanceDeleteBecauseNoModule         = plans.ResourceInstanceDeleteBecauseNoModule
	ResourceInstanceDeleteBecauseNoMoveTarget     = plans.ResourceInstanceDeleteBecauseNoMoveTarget
	ResourceInstanceReadBecauseConfigUnknown      = plans.ResourceInstanceReadBecauseConfigUnknown
	ResourceInstanceReadBecauseDependencyPending  = plans.ResourceInstanceReadBecauseDependencyPending
	ResourceInstanceReadBecauseCheckNested        = plans.ResourceInstanceReadBecauseCheckNested
)

// Constructors and helpers.
var (
	NewDynamicValue = plans.NewDynamicValue
	NewChanges      = plans.NewChanges
)
