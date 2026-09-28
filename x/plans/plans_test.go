// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package plans

import (
	"testing"

	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/zclconf/go-cty/cty"
)

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

// TestNewBackend pins that the backend a plan records carries its type, the
// workspace, and a configuration that decodes back against its schema.
func TestNewBackend(t *testing.T) {
	schema := &configschema.Block{Attributes: map[string]*configschema.Attribute{
		"path": {Type: cty.String, Optional: true},
	}}
	config := cty.ObjectVal(map[string]cty.Value{"path": cty.StringVal("terraform.tfstate")})

	var b *Backend
	b, err := NewBackend("local", config, schema, "prod")
	if err != nil {
		t.Fatal(err)
	}
	if b.Type != "local" || b.Workspace != "prod" {
		t.Errorf("backend = %s/%s", b.Type, b.Workspace)
	}
	got, err := b.Config.Decode(schema.ImpliedType())
	if err != nil {
		t.Fatal(err)
	}
	if !got.RawEquals(config) {
		t.Errorf("config round-trips as %#v", got)
	}
}
