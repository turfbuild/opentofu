// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package grpcwrap_test

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/msgpack"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/opentofu/opentofu/internal/builtin/providers/tf"
	"github.com/opentofu/opentofu/internal/grpcwrap"
	"github.com/opentofu/opentofu/internal/plugin"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfplugin5"
)

// servedBuiltin serves the built-in terraform provider through grpcwrap and
// returns OpenTofu's own client for it, so a test drives the server half with
// the client half it has to agree with.
func servedBuiltin(t *testing.T) providers.Interface {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	tfplugin5.RegisterProviderServer(srv, grpcwrap.Provider(tf.NewProvider()))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	client, err := (&plugin.GRPCProviderPlugin{}).GRPCClient(context.Background(), nil, conn)
	if err != nil {
		t.Fatal(err)
	}
	p := client.(*plugin.GRPCProvider)
	p.SchemaCache = providers.NewSchemaCache()
	return p
}

// TestServedFunctions pins the built-in provider's functions across the
// protocol: listed in the schema with their signatures, called with a dynamic
// argument and a dynamic result, and a provider error delivered without an
// argument index — which OpenTofu's client then lays on the first argument.
func TestServedFunctions(t *testing.T) {
	ctx := context.Background()
	p := servedBuiltin(t)

	schema := p.GetProviderSchema(ctx)
	if schema.Diagnostics.HasErrors() {
		t.Fatal(schema.Diagnostics.Err())
	}
	for _, name := range []string{"decode_tfvars", "encode_tfvars", "encode_expr"} {
		if _, ok := schema.Functions[name]; !ok {
			t.Errorf("schema lists no %s", name)
		}
	}
	if spec := schema.Functions["encode_expr"]; !spec.Return.Equals(cty.String) || len(spec.Parameters) != 1 ||
		spec.Parameters[0].Name != "expr" || !spec.Parameters[0].Type.Equals(cty.DynamicPseudoType) {
		t.Errorf("encode_expr's signature: %#v", spec)
	}
	funcs := p.GetFunctions(ctx)
	if funcs.Diagnostics.HasErrors() || len(funcs.Functions) != len(schema.Functions) {
		t.Errorf("GetFunctions: %d functions, %v", len(funcs.Functions), funcs.Diagnostics.Err())
	}

	encoded := p.CallFunction(ctx, providers.CallFunctionRequest{
		Name:      "encode_expr",
		Arguments: []cty.Value{cty.ObjectVal(map[string]cty.Value{"a": cty.NumberIntVal(1)})},
	})
	if encoded.Error != nil {
		t.Fatal(encoded.Error)
	}
	if got := encoded.Result.AsString(); got != "{\n  a = 1\n}" {
		t.Errorf("encode_expr = %q", got)
	}

	decoded := p.CallFunction(ctx, providers.CallFunctionRequest{
		Name:      "decode_tfvars",
		Arguments: []cty.Value{cty.StringVal("a = 1\nb = \"x\"\n")},
	})
	if decoded.Error != nil {
		t.Fatal(decoded.Error)
	}
	want := cty.ObjectVal(map[string]cty.Value{"a": cty.NumberIntVal(1), "b": cty.StringVal("x")})
	if !decoded.Result.RawEquals(want) {
		t.Errorf("decode_tfvars = %#v", decoded.Result)
	}

	refused := p.CallFunction(ctx, providers.CallFunctionRequest{
		Name:      "encode_tfvars",
		Arguments: []cty.Value{cty.StringVal("x")},
	})
	var argErr *providers.CallFunctionArgumentError
	if !errors.As(refused.Error, &argErr) || argErr.FunctionArgument != 0 || argErr.Text != "invalid input: must be an object" {
		t.Errorf("encode_tfvars(\"x\") error: %#v", refused.Error)
	}
}

// TestServedFunctionErrors pins the server half directly. An absent argument
// is the null OpenTofu's client leaves out for a parameter that admits one. A
// provider error carries no argument index, an argument that does not decode
// blames itself, and an unknown name or a wrong count is an index-less error
// rather than a panic.
func TestServedFunctionErrors(t *testing.T) {
	ctx := context.Background()
	srv := grpcwrap.Provider(tf.NewProvider())

	null, err := srv.CallFunction(ctx, &tfplugin5.CallFunction_Request{Name: "encode_expr", Arguments: []*tfplugin5.DynamicValue{nil}})
	if err != nil || null.Error != nil {
		t.Fatalf("encode_expr(<absent>): %v, %#v", err, null.Error)
	}
	if got, err := msgpack.Unmarshal(null.Result.Msgpack, cty.String); err != nil || got.AsString() != "null" {
		t.Errorf("encode_expr(<absent>) = %#v, %v", got, err)
	}

	str := func(s string) *tfplugin5.DynamicValue {
		return &tfplugin5.DynamicValue{Json: []byte(`"` + s + `"`)}
	}
	for name, tc := range map[string]struct {
		req     *tfplugin5.CallFunction_Request
		wantArg *int64
	}{
		"provider error": {
			req: &tfplugin5.CallFunction_Request{Name: "decode_tfvars", Arguments: []*tfplugin5.DynamicValue{str("a = ")}},
		},
		"undecodable argument": {
			req:     &tfplugin5.CallFunction_Request{Name: "decode_tfvars", Arguments: []*tfplugin5.DynamicValue{{Msgpack: []byte{0xc1}}}},
			wantArg: new(int64),
		},
		"unknown function": {
			req: &tfplugin5.CallFunction_Request{Name: "nope", Arguments: []*tfplugin5.DynamicValue{str("x")}},
		},
		"too many arguments": {
			req: &tfplugin5.CallFunction_Request{Name: "decode_tfvars", Arguments: []*tfplugin5.DynamicValue{str("x"), str("y")}},
		},
		"too few arguments": {
			req: &tfplugin5.CallFunction_Request{Name: "decode_tfvars"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			resp, err := srv.CallFunction(ctx, tc.req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.Error == nil || resp.Error.Text == "" {
				t.Fatalf("no error: %#v", resp)
			}
			switch {
			case tc.wantArg == nil && resp.Error.FunctionArgument != nil:
				t.Errorf("blames argument %d: %s", *resp.Error.FunctionArgument, resp.Error.Text)
			case tc.wantArg != nil && (resp.Error.FunctionArgument == nil || *resp.Error.FunctionArgument != *tc.wantArg):
				t.Errorf("blames %v, want %d: %s", resp.Error.FunctionArgument, *tc.wantArg, resp.Error.Text)
			}
		})
	}
}
