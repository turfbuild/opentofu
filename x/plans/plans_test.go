// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package plans

import "testing"

// TestActionVocabulary pins that the action and reason enums are whole:
// ForgetThenCreate is what a replace of a `destroy = false` resource plans,
// and the two forget reasons are what jsonplan renders as its action_reason.
func TestActionVocabulary(t *testing.T) {
	if got := ForgetThenCreate.String(); got != "ForgetThenCreate" {
		t.Errorf("ForgetThenCreate renders as %s", got)
	}
	for reason, want := range map[ResourceInstanceChangeActionReason]string{
		ResourceInstanceForgotBecauseLifecycleDestroyInState:  "ResourceInstanceForgotBecauseLifecycleDestroyInState",
		ResourceInstanceForgotBecauseLifecycleDestroyInConfig: "ResourceInstanceForgotBecauseLifecycleDestroyInConfig",
	} {
		if reason.String() != want {
			t.Errorf("reason %q renders as %s, want %s", rune(reason), reason, want)
		}
	}
}
