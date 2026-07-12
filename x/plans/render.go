// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package plans

import (
	"fmt"

	tofuplans "github.com/opentofu/opentofu/internal/plans"
)

// JSON action strings, matching internal/command/jsonentities so a consumer's wire
// shape aligns with `tofu show -json` consumers.
const (
	JSONActionNoOp    = "noop"
	JSONActionCreate  = "create"
	JSONActionRead    = "read"
	JSONActionUpdate  = "update"
	JSONActionReplace = "replace"
	JSONActionDelete  = "delete"
	JSONActionForget  = "forget"
	JSONActionOpen    = "open"
)

// Display symbols for interactive prompts and logs. Kept as a
// presentation-only helper; wire JSON uses the JSON*-strings.
const (
	DisplayNoOp    = '='
	DisplayCreate  = '+'
	DisplayRead    = '←'
	DisplayUpdate  = '~'
	DisplayDelete  = '-'
	DisplayForget  = '.'
	DisplayOpen    = '⁐'
	DisplayReplace = '±' // ambiguous on its own; callers may pick ∓ for delete-then-create
)

// JSONAction maps a plans.Action to the lowercase string used in tofu's
// JSON plan output. Both CreateThenDelete and DeleteThenCreate collapse
// to "replace"; callers needing to distinguish the order should read the
// underlying Action enum (or the change's create_before_destroy bit).
func JSONAction(a Action) string {
	switch a {
	case tofuplans.NoOp:
		return JSONActionNoOp
	case tofuplans.Create:
		return JSONActionCreate
	case tofuplans.Read:
		return JSONActionRead
	case tofuplans.Update:
		return JSONActionUpdate
	case tofuplans.Delete:
		return JSONActionDelete
	case tofuplans.CreateThenDelete, tofuplans.DeleteThenCreate:
		return JSONActionReplace
	case tofuplans.Forget:
		return JSONActionForget
	default:
		return fmt.Sprintf("unknown(%q)", rune(a))
	}
}

// ParseJSONAction reverses JSONAction. "replace" maps to DeleteThenCreate by
// default; callers tracking create_before_destroy elsewhere should adjust.
func ParseJSONAction(s string) (Action, error) {
	switch s {
	case JSONActionNoOp:
		return tofuplans.NoOp, nil
	case JSONActionCreate:
		return tofuplans.Create, nil
	case JSONActionRead:
		return tofuplans.Read, nil
	case JSONActionUpdate:
		return tofuplans.Update, nil
	case JSONActionDelete:
		return tofuplans.Delete, nil
	case JSONActionReplace:
		return tofuplans.DeleteThenCreate, nil
	case JSONActionForget:
		return tofuplans.Forget, nil
	default:
		return 0, fmt.Errorf("unknown JSON action %q", s)
	}
}

// DisplaySymbol returns the rune used in interactive prompts and logs. For replace
// actions the caller picks ± (CreateThenDelete) or ∓ (DeleteThenCreate)
// explicitly via the underlying Action enum.
func DisplaySymbol(a Action) rune {
	switch a {
	case tofuplans.NoOp:
		return DisplayNoOp
	case tofuplans.Create:
		return DisplayCreate
	case tofuplans.Read:
		return DisplayRead
	case tofuplans.Update:
		return DisplayUpdate
	case tofuplans.Delete:
		return DisplayDelete
	case tofuplans.CreateThenDelete:
		return rune(tofuplans.CreateThenDelete) // '±'
	case tofuplans.DeleteThenCreate:
		return rune(tofuplans.DeleteThenCreate) // '∓'
	case tofuplans.Forget:
		return DisplayForget
	default:
		return '?'
	}
}

// JSONActionReason maps a ResourceInstanceChangeActionReason to the string
// vocabulary used by jsonentities.ChangeReason.
func JSONActionReason(r ResourceInstanceChangeActionReason) string {
	switch r {
	case tofuplans.ResourceInstanceChangeNoReason:
		return ""
	case tofuplans.ResourceInstanceReplaceBecauseTainted:
		return "tainted"
	case tofuplans.ResourceInstanceReplaceByRequest:
		return "replace_by_request"
	case tofuplans.ResourceInstanceReplaceByTriggers:
		return "replace_triggered_by"
	case tofuplans.ResourceInstanceReplaceBecauseCannotUpdate:
		return "cannot_update"
	case tofuplans.ResourceInstanceDeleteBecauseNoResourceConfig:
		return "delete_because_no_resource_config"
	case tofuplans.ResourceInstanceDeleteBecauseWrongRepetition:
		return "delete_because_wrong_repetition"
	case tofuplans.ResourceInstanceDeleteBecauseCountIndex:
		return "delete_because_count_index"
	case tofuplans.ResourceInstanceDeleteBecauseEachKey:
		return "delete_because_each_key"
	case tofuplans.ResourceInstanceDeleteBecauseEnabledFalse:
		return "delete_because_enabled_false"
	case tofuplans.ResourceInstanceDeleteBecauseNoModule:
		return "delete_because_no_module"
	case tofuplans.ResourceInstanceDeleteBecauseNoMoveTarget:
		return "delete_because_no_move_target"
	case tofuplans.ResourceInstanceReadBecauseConfigUnknown:
		return "read_because_config_unknown"
	case tofuplans.ResourceInstanceReadBecauseDependencyPending:
		return "read_because_dependency_pending"
	case tofuplans.ResourceInstanceReadBecauseCheckNested:
		return "read_because_check_nested"
	default:
		return "unknown"
	}
}
