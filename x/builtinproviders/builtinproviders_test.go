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

	tfplugin "github.com/opentofu/opentofu/internal/plugin"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfplugin5"
)

func TestNames(t *testing.T) {
	got := Names()
	if len(got) != 1 || got[0] != "terraform" {
		t.Fatalf("Names() = %v, want [terraform]", got)
	}
	if addr := Addr("terraform"); addr.String() != "terraform.io/builtin/terraform" || !addr.IsBuiltIn() {
		t.Fatalf("Addr(terraform) = %q (builtin=%v)", addr, addr.IsBuiltIn())
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

// TestServeHelperProcess is the child half of TestServeRoundTrip: under the
// marker it serves the terraform provider until the client hangs up and then
// exits. It is a no-op as a test in its own right.
func TestServeHelperProcess(t *testing.T) {
	if os.Getenv(helperEnv) != "1" {
		return
	}
	if err := Serve("terraform"); err != nil {
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
