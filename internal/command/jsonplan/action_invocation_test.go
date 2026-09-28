// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsonplan

import (
	"encoding/json"
	"testing"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/plans"
)

// TestActionInvocationJSONShape pins one lifecycle invocation's whole entry:
// Terraform's key set and spellings, and the configuration as values plus the
// unknown and sensitive trees.
func TestActionInvocationJSONShape(t *testing.T) {
	config := cty.ObjectVal(map[string]cty.Value{
		"message": cty.StringVal("ready?"),
		"token":   cty.StringVal("s3cr3t").Mark(marks.Sensitive),
		"target":  cty.UnknownVal(cty.String),
		"tags":    cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}),
		"unset":   cty.NullVal(cty.String),
	})
	got, err := MarshalActionInvocations([]*plans.ActionInvocationInstanceSrc{{
		Addr:     "module.db.action.turf_confirm.gate[0]",
		Type:     "turf_confirm",
		Name:     "gate",
		Provider: addrs.Provider{Hostname: "terraform.io", Namespace: "builtin", Type: "turf"},
		Config:   config,
		LifecycleTrigger: &plans.ActionLifecycleTrigger{
			TriggeringResourceAddr: "module.db.aws_db_instance.main",
			Event:                  "before_create",
			BlockIndex:             2,
			ListIndex:              1,
			OnFailure:              "taint",
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"address":"module.db.action.turf_confirm.gate[0]","type":"turf_confirm","name":"gate",` +
		`"config_values":{"message":"ready?","tags":["a","b"],"token":"s3cr3t","unset":null},` +
		`"config_sensitive":{"tags":[false,false],"token":true},` +
		`"config_unknown":{"tags":[false,false],"target":true},` +
		`"provider_name":"terraform.io/builtin/turf",` +
		`"lifecycle_action_trigger":{"triggering_resource_address":"module.db.aws_db_instance.main",` +
		`"action_trigger_event":"BeforeCreate","action_trigger_block_index":2,"actions_list_index":1,` +
		`"on_failure":"ActionOnFailureTaint"}}]`
	if string(raw) != want {
		t.Errorf("wrong entry\ngot:  %s\nwant: %s", raw, want)
	}
}

// TestActionInvocationEmptyConfigAndInvoke: an action with no configuration
// renders null, {}, {}; an imperative invocation carries invoke_action_trigger
// (empty, or naming its caller) and no lifecycle trigger; on_failure unset
// spells halt.
func TestActionInvocationEmptyConfigAndInvoke(t *testing.T) {
	got, err := MarshalActionInvocations([]*plans.ActionInvocationInstanceSrc{
		{Addr: "action.mock_act.b", Type: "mock_act", Name: "b",
			InvokeTrigger: &plans.ActionInvokeTrigger{CallingResourceAddr: "mock_thing.x"}},
		{Addr: "action.mock_act.a", Type: "mock_act", Name: "a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(got)
	want := `[{"address":"action.mock_act.a","type":"mock_act","name":"a","config_values":null,` +
		`"config_sensitive":{},"config_unknown":{},"provider_name":"","invoke_action_trigger":{}},` +
		`{"address":"action.mock_act.b","type":"mock_act","name":"b","config_values":null,` +
		`"config_sensitive":{},"config_unknown":{},"provider_name":"",` +
		`"invoke_action_trigger":{"calling_resource_address":"mock_thing.x"}}]`
	if string(raw) != want {
		t.Errorf("wrong entries\ngot:  %s\nwant: %s", raw, want)
	}
	if got := actionOnFailureJSONName(""); got != "ActionOnFailureHalt" {
		t.Errorf("unset on_failure spelled %q", got)
	}
}

// TestActionInvocationOrder: lifecycle invocations list by triggering
// resource, then lifecycle order of the event (not its spelling), then block,
// then list position.
func TestActionInvocationOrder(t *testing.T) {
	inv := func(addr, caller, event string, block, list int) *plans.ActionInvocationInstanceSrc {
		return &plans.ActionInvocationInstanceSrc{Addr: addr, Type: "t", Name: "n",
			LifecycleTrigger: &plans.ActionLifecycleTrigger{TriggeringResourceAddr: caller, Event: event,
				BlockIndex: block, ListIndex: list}}
	}
	got, err := MarshalActionInvocations([]*plans.ActionInvocationInstanceSrc{
		inv("action.t.6", "r.b", "before_create", 0, 0),
		inv("action.t.5", "r.a", "after_destroy", 0, 0),
		inv("action.t.4", "r.a", "after_create", 1, 1),
		inv("action.t.3", "r.a", "after_create", 1, 0),
		inv("action.t.2", "r.a", "after_create", 0, 3),
		inv("action.t.1", "r.a", "before_create", 4, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, g := range got {
		order = append(order, g.Address)
	}
	want := []string{"action.t.1", "action.t.2", "action.t.3", "action.t.4", "action.t.5", "action.t.6"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

// TestActionInvocationProviderName: an invocation names its provider through
// addrs.Provider.String(), the rendering resource_changes uses, so one
// document cannot spell one provider two ways. A non-default host is the
// load-bearing case: a default-registry fixture cannot tell a renderer that
// qualifies the address from one that drops the host.
func TestActionInvocationProviderName(t *testing.T) {
	provider := addrs.Provider{Hostname: "example.com", Namespace: "example", Type: "mock"}
	got, err := MarshalActionInvocations([]*plans.ActionInvocationInstanceSrc{{
		Addr: "action.mock_act.one", Type: "mock_act", Name: "one", Provider: provider,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].ProviderName != "example.com/example/mock" || got[0].ProviderName != provider.String() {
		t.Errorf("provider_name = %q", got[0].ProviderName)
	}
}
