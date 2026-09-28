// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package builtinproviders

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/hashicorp/go-hclog"
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/msgpack"
	"google.golang.org/grpc"

	"github.com/opentofu/opentofu/internal/configs/configschema"
	tfplugin "github.com/opentofu/opentofu/internal/plugin"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfplugin5"
)

func TestNames(t *testing.T) {
	got := Names()
	if len(got) != 1 || got[0] != "terraform" {
		t.Fatalf("Names() = %v, want [terraform]", got)
	}
}

func TestServeUnknownName(t *testing.T) {
	// The one path on which Serve returns: it must do so before go-plugin
	// takes over the process, or the test binary would hang on a handshake.
	err := Serve("nonesuch")
	if err == nil {
		t.Fatal("Serve(nonesuch) returned nil; it should refuse before serving")
	}
	if _, err := serverFor("nonesuch"); err == nil {
		t.Fatal("serverFor(nonesuch) returned a server")
	}
}

// TestServerSchema drives the wrapped server in-process: the terraform_data
// schema is the one the fixture corpus will see, an empty configuration is
// accepted (the provider has no configuration block), and a create plan
// leaves id and output unknown.
func TestServerSchema(t *testing.T) {
	ctx := context.Background()
	server, err := serverFor("terraform")
	if err != nil {
		t.Fatal(err)
	}
	schemaResp, err := server.GetSchema(ctx, &tfplugin5.GetProviderSchema_Request{})
	if err != nil {
		t.Fatal(err)
	}
	if len(schemaResp.Diagnostics) > 0 {
		t.Fatalf("GetSchema diagnostics: %v", schemaResp.Diagnostics)
	}
	rs, ok := schemaResp.ResourceSchemas["terraform_data"]
	if !ok {
		t.Fatalf("resource schemas %v lack terraform_data", schemaResp.ResourceSchemas)
	}
	attrs := map[string]bool{}
	for _, a := range rs.Block.Attributes {
		attrs[a.Name] = true
	}
	for _, want := range []string{"input", "output", "triggers_replace", "id"} {
		if !attrs[want] {
			t.Errorf("terraform_data schema lacks %q (has %v)", want, attrs)
		}
	}

	// No configuration block, so the client sends the empty value a
	// schema-less provider gets; the wrapper decodes it against EmptyObject
	// and the provider's no-op Configure accepts it.
	cfgResp, err := server.Configure(ctx, &tfplugin5.Configure_Request{TerraformVersion: "1.12.0"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfgResp.Diagnostics) > 0 {
		t.Fatalf("Configure diagnostics: %v", cfgResp.Diagnostics)
	}

	ty := cty.Object(map[string]cty.Type{
		"input":            cty.DynamicPseudoType,
		"output":           cty.DynamicPseudoType,
		"triggers_replace": cty.DynamicPseudoType,
		"id":               cty.String,
	})
	proposed := cty.ObjectVal(map[string]cty.Value{
		"input":            cty.StringVal("one"),
		"output":           cty.NullVal(cty.DynamicPseudoType),
		"triggers_replace": cty.NullVal(cty.DynamicPseudoType),
		"id":               cty.NullVal(cty.String),
	})
	proposedMP, err := msgpack.Marshal(proposed, ty)
	if err != nil {
		t.Fatal(err)
	}
	planResp, err := server.PlanResourceChange(ctx, &tfplugin5.PlanResourceChange_Request{
		TypeName:         "terraform_data",
		PriorState:       &tfplugin5.DynamicValue{Msgpack: mustMsgpack(t, cty.NullVal(ty), ty)},
		ProposedNewState: &tfplugin5.DynamicValue{Msgpack: proposedMP},
		Config:           &tfplugin5.DynamicValue{Msgpack: proposedMP},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(planResp.Diagnostics) > 0 {
		t.Fatalf("PlanResourceChange diagnostics: %v", planResp.Diagnostics)
	}
	planned, err := msgpack.Unmarshal(planResp.PlannedState.Msgpack, ty)
	if err != nil {
		t.Fatal(err)
	}
	if planned.GetAttr("id").IsKnown() {
		t.Errorf("a create plan should leave id unknown; got %#v", planned.GetAttr("id"))
	}
	if planned.GetAttr("output").IsKnown() {
		t.Errorf("a create plan should leave output unknown; got %#v", planned.GetAttr("output"))
	}
}

func mustMsgpack(t *testing.T, v cty.Value, ty cty.Type) []byte {
	t.Helper()
	b, err := msgpack.Marshal(v, ty)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// helperEnv marks the re-executed test binary as the plugin child. Args
// select the helper test; the environment is what tells it to serve rather
// than to pass trivially when `go test` runs it in the ordinary way.
const helperEnv = "TOFU_X_BUILTINPROVIDERS_SERVE"

// TestServeHelperProcess is the child half of the round-trip tests: under the
// marker it serves the terraform provider, or the test's action-only one,
// until the client hangs up and then exits. It is a no-op as a test in its
// own right.
func TestServeHelperProcess(t *testing.T) {
	var err error
	switch os.Getenv(helperEnv) {
	case "1":
		err = Serve("terraform")
	case helperActions:
		err = ServeActions(testActions())
	default:
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}

// TestServeRoundTrip launches this test binary as a plugin child through
// go-plugin's own client — the handshake, cookie and protocol negotiation a
// plugin client performs against a downloaded provider binary — and reads
// the schema back over the wire. It is the proof that Serve produces a
// process a stock plugin client accepts; a library host's own client is then
// only another client.
func TestServeRoundTrip(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestServeHelperProcess$")
	cmd.Env = append(os.Environ(), helperEnv+"=1")
	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig:  tfplugin.Handshake,
		Logger:           hclog.NewNullLogger(),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Cmd:              cmd,
		VersionedPlugins: tfplugin.VersionedPlugins,
	})
	t.Cleanup(client.Kill)

	rpcClient, err := client.Client()
	if err != nil {
		t.Fatalf("plugin client: %v", err)
	}
	if v := client.NegotiatedVersion(); v != 5 {
		t.Fatalf("negotiated protocol %d, want 5 (the wrapper is a tfplugin5 server)", v)
	}
	raw, err := rpcClient.Dispense(tfplugin.ProviderPluginName)
	if err != nil {
		t.Fatalf("dispense: %v", err)
	}
	p, ok := raw.(*tfplugin.GRPCProvider)
	if !ok {
		t.Fatalf("dispensed %T, want *plugin.GRPCProvider", raw)
	}
	p.PluginClient = client
	p.SchemaCache = providers.NewSchemaCache()

	resp := p.GetProviderSchema(context.Background())
	if resp.Diagnostics.HasErrors() {
		t.Fatalf("GetProviderSchema over the wire: %s", resp.Diagnostics.Err())
	}
	rs, ok := resp.ResourceTypes["terraform_data"]
	if !ok {
		t.Fatalf("resource types over the wire %v lack terraform_data", resp.ResourceTypes)
	}
	if got := rs.Block.Attributes["input"].Type; got != cty.DynamicPseudoType {
		t.Errorf("input type over the wire = %#v, want DynamicPseudoType", got)
	}
	if err := p.Close(context.Background()); err != nil {
		t.Errorf("close: %v", err)
	}
}

// helperActions is the marker value under which the helper process serves
// testActions through ServeActions instead of the terraform provider.
const helperActions = "actions"

// testActions is a two-type action-only provider: one type with a rule of
// its own, one the schema alone judges.
func testActions() map[string]Action {
	return map[string]Action{
		"test_gate": {
			Schema: &configschema.Block{Attributes: map[string]*configschema.Attribute{
				"message": {Type: cty.String, Required: true},
				"mode":    {Type: cty.String, Optional: true},
			}},
			Validate: func(config cty.Value) ([]string, error) {
				mode := config.GetAttr("mode")
				if !mode.IsKnown() || mode.IsNull() {
					return nil, nil
				}
				switch mode.AsString() {
				case "strict":
					return nil, nil
				case "lenient":
					return []string{"lenient gates are advisory"}, nil
				}
				return nil, cty.Path{}.GetAttr("mode").NewErrorf("mode must be strict or lenient, not %q", mode.AsString())
			},
		},
		"test_note": {
			Schema: &configschema.Block{Attributes: map[string]*configschema.Attribute{
				"text": {Type: cty.String, Required: true},
			}},
		},
	}
}

func gateConfig(t *testing.T, message, mode cty.Value) *tfplugin5.DynamicValue {
	t.Helper()
	ty := cty.Object(map[string]cty.Type{"message": cty.String, "mode": cty.String})
	return &tfplugin5.DynamicValue{Msgpack: mustMsgpack(t, cty.ObjectVal(map[string]cty.Value{"message": message, "mode": mode}), ty)}
}

func TestActionServerRefusesBadDefinitions(t *testing.T) {
	if err := ServeActions(nil); err == nil {
		t.Error("ServeActions(nil) returned nil; it should refuse before serving")
	}
	if err := ServeActions(map[string]Action{"test_x": {}}); err == nil {
		t.Error("ServeActions with a schema-less action returned nil; it should refuse before serving")
	}
}

// TestActionServer drives the action-only server in-process through
// everything a plugin client asks of an action: the schema, a configure with
// no configuration, the two judgements, and the invoke it declines.
func TestActionServer(t *testing.T) {
	ctx := context.Background()
	server, err := actionServerFor(testActions())
	if err != nil {
		t.Fatal(err)
	}

	schemaResp, err := server.GetSchema(ctx, &tfplugin5.GetProviderSchema_Request{})
	if err != nil {
		t.Fatal(err)
	}
	if schemaResp.Provider.GetBlock() == nil || schemaResp.ProviderMeta.GetBlock() == nil {
		t.Fatalf("provider and provider-meta blocks must be present and empty; got %v / %v", schemaResp.Provider, schemaResp.ProviderMeta)
	}
	if n := len(schemaResp.ResourceSchemas) + len(schemaResp.DataSourceSchemas) + len(schemaResp.EphemeralResourceSchemas); n != 0 {
		t.Errorf("an action-only provider declared %d other schemas", n)
	}
	gate := schemaResp.ActionSchemas["test_gate"].GetSchema().GetBlock()
	if gate == nil || schemaResp.ActionSchemas["test_note"] == nil {
		t.Fatalf("action schemas %v lack the two types", schemaResp.ActionSchemas)
	}
	required := map[string]bool{}
	for _, a := range gate.Attributes {
		required[a.Name] = a.Required
	}
	if !required["message"] || required["mode"] || len(required) != 2 {
		t.Errorf("test_gate attributes (name → required) = %v, want message required and mode optional", required)
	}

	if resp, err := server.Configure(ctx, &tfplugin5.Configure_Request{}); err != nil || len(resp.Diagnostics) > 0 {
		t.Fatalf("Configure with no configuration: %v, %v", err, resp.GetDiagnostics())
	}
	if resp, err := server.PrepareProviderConfig(ctx, &tfplugin5.PrepareProviderConfig_Request{}); err != nil || len(resp.Diagnostics) > 0 {
		t.Fatalf("PrepareProviderConfig with no configuration: %v, %v", err, resp.GetDiagnostics())
	}

	judgements := []struct {
		name         string
		typeName     string
		config       *tfplugin5.DynamicValue
		wantSeverity tfplugin5.Diagnostic_Severity // 0: no diagnostics
		wantAttr     string
	}{
		{"accepted", "test_gate", gateConfig(t, cty.StringVal("go?"), cty.StringVal("strict")), 0, ""},
		{"unset optional", "test_gate", gateConfig(t, cty.StringVal("go?"), cty.NullVal(cty.String)), 0, ""},
		{"unknown passes", "test_gate", gateConfig(t, cty.UnknownVal(cty.String), cty.UnknownVal(cty.String)), 0, ""},
		{"warning", "test_gate", gateConfig(t, cty.StringVal("go?"), cty.StringVal("lenient")), tfplugin5.Diagnostic_WARNING, ""},
		{"refused, at its attribute", "test_gate", gateConfig(t, cty.StringVal("go?"), cty.StringVal("maybe")), tfplugin5.Diagnostic_ERROR, "mode"},
		{"no rule of its own", "test_note", &tfplugin5.DynamicValue{Msgpack: mustMsgpack(t,
			cty.ObjectVal(map[string]cty.Value{"text": cty.StringVal("hi")}), cty.Object(map[string]cty.Type{"text": cty.String}))}, 0, ""},
		{"wrong shape", "test_note", gateConfig(t, cty.StringVal("go?"), cty.StringVal("strict")), tfplugin5.Diagnostic_ERROR, ""},
		{"unknown type", "test_nonesuch", gateConfig(t, cty.StringVal("go?"), cty.StringVal("strict")), tfplugin5.Diagnostic_ERROR, ""},
	}
	for _, j := range judgements {
		check := func(rpc string, diags []*tfplugin5.Diagnostic) {
			t.Helper()
			if j.wantSeverity == 0 {
				if len(diags) > 0 {
					t.Errorf("%s %s: diagnostics %v, want none", rpc, j.name, diags)
				}
				return
			}
			if len(diags) != 1 || diags[0].Severity != j.wantSeverity {
				t.Errorf("%s %s: diagnostics %v, want one of severity %v", rpc, j.name, diags, j.wantSeverity)
				return
			}
			if j.wantAttr != "" {
				steps := diags[0].GetAttribute().GetSteps()
				if len(steps) != 1 || steps[0].GetAttributeName() != j.wantAttr {
					t.Errorf("%s %s: diagnostic names %v, want attribute %q", rpc, j.name, diags[0].GetAttribute(), j.wantAttr)
				}
			}
		}
		v, err := server.ValidateActionConfig(ctx, &tfplugin5.ValidateActionConfig_Request{TypeName: j.typeName, Config: j.config})
		if err != nil {
			t.Fatalf("ValidateActionConfig %s: %v", j.name, err)
		}
		check("ValidateActionConfig", v.Diagnostics)
		p, err := server.PlanAction(ctx, &tfplugin5.PlanAction_Request{ActionType: j.typeName, Config: j.config})
		if err != nil {
			t.Fatalf("PlanAction %s: %v", j.name, err)
		}
		check("PlanAction", p.Diagnostics)
		if p.Deferred != nil {
			t.Errorf("PlanAction %s deferred (%v); an action-only provider never does", j.name, p.Deferred)
		}
	}

	// The invoke is declined, as a completed event carrying an error: the
	// host that asked for these definitions answers the action itself.
	stream := &captureInvoke{ctx: ctx}
	if err := server.InvokeAction(&tfplugin5.InvokeAction_Request{ActionType: "test_gate"}, stream); err != nil {
		t.Fatalf("InvokeAction: %v", err)
	}
	if len(stream.events) != 1 {
		t.Fatalf("InvokeAction sent %d events, want the one completion", len(stream.events))
	}
	done := stream.events[0].GetCompleted()
	if done == nil || len(done.Diagnostics) != 1 || done.Diagnostics[0].Severity != tfplugin5.Diagnostic_ERROR {
		t.Fatalf("InvokeAction completed with %v, want one error diagnostic", stream.events[0])
	}
}

// captureInvoke is the server half of the InvokeAction stream, kept in
// memory.
type captureInvoke struct {
	grpc.ServerStream
	ctx    context.Context
	events []*tfplugin5.InvokeAction_Event
}

func (c *captureInvoke) Context() context.Context { return c.ctx }

func (c *captureInvoke) Send(e *tfplugin5.InvokeAction_Event) error {
	c.events = append(c.events, e)
	return nil
}

// TestServeActionsRoundTrip is TestServeRoundTrip for the action-only
// server: a stock plugin client launches the child, negotiates protocol 5
// and loads its schema — which OpenTofu's own client reads without the
// actions, having no place to put them — and then the raw protocol client
// on the same connection reads the action schemas and has a configuration
// judged, which is what a host that knows actions does.
func TestServeActionsRoundTrip(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestServeHelperProcess$")
	cmd.Env = append(os.Environ(), helperEnv+"="+helperActions)
	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig:  tfplugin.Handshake,
		Logger:           hclog.NewNullLogger(),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Cmd:              cmd,
		VersionedPlugins: tfplugin.VersionedPlugins,
	})
	t.Cleanup(client.Kill)

	rpcClient, err := client.Client()
	if err != nil {
		t.Fatalf("plugin client: %v", err)
	}
	if v := client.NegotiatedVersion(); v != 5 {
		t.Fatalf("negotiated protocol %d, want 5", v)
	}
	raw, err := rpcClient.Dispense(tfplugin.ProviderPluginName)
	if err != nil {
		t.Fatalf("dispense: %v", err)
	}
	p, ok := raw.(*tfplugin.GRPCProvider)
	if !ok {
		t.Fatalf("dispensed %T, want *plugin.GRPCProvider", raw)
	}
	p.PluginClient = client
	p.SchemaCache = providers.NewSchemaCache()
	if resp := p.GetProviderSchema(context.Background()); resp.Diagnostics.HasErrors() {
		t.Fatalf("GetProviderSchema over the wire: %s", resp.Diagnostics.Err())
	}

	grpcClient, ok := rpcClient.(*goplugin.GRPCClient)
	if !ok {
		t.Fatalf("plugin protocol client is %T, want *plugin.GRPCClient", rpcClient)
	}
	wire := tfplugin5.NewProviderClient(grpcClient.Conn)
	schemaResp, err := wire.GetSchema(context.Background(), &tfplugin5.GetProviderSchema_Request{})
	if err != nil {
		t.Fatalf("GetSchema over the wire: %v", err)
	}
	if schemaResp.ActionSchemas["test_gate"] == nil || schemaResp.ActionSchemas["test_note"] == nil {
		t.Fatalf("action schemas over the wire %v lack the two types", schemaResp.ActionSchemas)
	}
	v, err := wire.ValidateActionConfig(context.Background(), &tfplugin5.ValidateActionConfig_Request{
		TypeName: "test_gate", Config: gateConfig(t, cty.StringVal("go?"), cty.StringVal("maybe")),
	})
	if err != nil {
		t.Fatalf("ValidateActionConfig over the wire: %v", err)
	}
	if len(v.Diagnostics) != 1 || v.Diagnostics[0].Severity != tfplugin5.Diagnostic_ERROR {
		t.Fatalf("ValidateActionConfig over the wire: %v, want the one refusal", v.Diagnostics)
	}
	if err := p.Close(context.Background()); err != nil {
		t.Errorf("close: %v", err)
	}
}
