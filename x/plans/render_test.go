// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package plans

import (
	"testing"

	tofuplans "github.com/opentofu/opentofu/internal/plans"
)

func TestJSONAction(t *testing.T) {
	cases := []struct {
		in   Action
		want string
	}{
		{tofuplans.NoOp, "noop"},
		{tofuplans.Create, "create"},
		{tofuplans.Read, "read"},
		{tofuplans.Update, "update"},
		{tofuplans.Delete, "delete"},
		{tofuplans.CreateThenDelete, "replace"},
		{tofuplans.DeleteThenCreate, "replace"},
		{tofuplans.Forget, "forget"},
	}
	for _, tc := range cases {
		if got := JSONAction(tc.in); got != tc.want {
			t.Errorf("JSONAction(%q) = %q, want %q", rune(tc.in), got, tc.want)
		}
	}
}

func TestParseJSONAction(t *testing.T) {
	// Round-trip every non-replace action via JSONAction -> ParseJSONAction.
	// Replace is asymmetric (collapses) and tested separately.
	roundTrip := []Action{
		tofuplans.NoOp, tofuplans.Create, tofuplans.Read, tofuplans.Update,
		tofuplans.Delete, tofuplans.Forget,
	}
	for _, a := range roundTrip {
		got, err := ParseJSONAction(JSONAction(a))
		if err != nil {
			t.Errorf("ParseJSONAction(JSONAction(%q)) error: %v", rune(a), err)
			continue
		}
		if got != a {
			t.Errorf("round-trip %q -> %q -> %q (want %q)", rune(a), JSONAction(a), rune(got), rune(a))
		}
	}

	// "replace" defaults to DeleteThenCreate. CreateThenDelete callers must
	// carry the create_before_destroy bit separately.
	got, err := ParseJSONAction("replace")
	if err != nil {
		t.Fatalf("ParseJSONAction(\"replace\") error: %v", err)
	}
	if got != tofuplans.DeleteThenCreate {
		t.Errorf("ParseJSONAction(\"replace\") = %q, want DeleteThenCreate", rune(got))
	}

	if _, err := ParseJSONAction("nope"); err == nil {
		t.Error("ParseJSONAction(\"nope\") expected error, got nil")
	}
}

func TestDisplaySymbol(t *testing.T) {
	cases := []struct {
		in   Action
		want rune
	}{
		{tofuplans.NoOp, '='},
		{tofuplans.Create, '+'},
		{tofuplans.Read, '←'},
		{tofuplans.Update, '~'},
		{tofuplans.Delete, '-'},
		{tofuplans.CreateThenDelete, '±'},
		{tofuplans.DeleteThenCreate, '∓'},
		{tofuplans.Forget, '.'},
	}
	for _, tc := range cases {
		if got := DisplaySymbol(tc.in); got != tc.want {
			t.Errorf("DisplaySymbol(%q) = %q, want %q", rune(tc.in), got, tc.want)
		}
	}
}

func TestJSONActionReason(t *testing.T) {
	cases := []struct {
		in   ResourceInstanceChangeActionReason
		want string
	}{
		{tofuplans.ResourceInstanceChangeNoReason, ""},
		{tofuplans.ResourceInstanceReplaceBecauseTainted, "tainted"},
		{tofuplans.ResourceInstanceReplaceByRequest, "replace_by_request"},
		{tofuplans.ResourceInstanceReplaceByTriggers, "replace_triggered_by"},
		{tofuplans.ResourceInstanceReplaceBecauseCannotUpdate, "cannot_update"},
		{tofuplans.ResourceInstanceDeleteBecauseNoResourceConfig, "delete_because_no_resource_config"},
		{tofuplans.ResourceInstanceDeleteBecauseCountIndex, "delete_because_count_index"},
		{tofuplans.ResourceInstanceDeleteBecauseEachKey, "delete_because_each_key"},
		{tofuplans.ResourceInstanceDeleteBecauseEnabledFalse, "delete_because_enabled_false"},
		{tofuplans.ResourceInstanceReadBecauseConfigUnknown, "read_because_config_unknown"},
	}
	for _, tc := range cases {
		if got := JSONActionReason(tc.in); got != tc.want {
			t.Errorf("JSONActionReason(%q) = %q, want %q", rune(tc.in), got, tc.want)
		}
	}
}

func TestAliasesCompile(t *testing.T) {
	// Compile-time check: confirm the type aliases are usable in declarations.
	var _ *Plan
	var _ *Changes
	var _ *ResourceInstanceChangeSrc
	var _ *OutputChangeSrc
	var _ DynamicValue
	var _ Action = Create
	var _ ResourceInstanceChangeActionReason = ResourceInstanceReplaceByRequest
	var _ Mode = NormalMode
	if NewDynamicValue == nil {
		t.Error("NewDynamicValue alias is nil")
	}
}
