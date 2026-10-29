// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package grpcwrap

// Downstream: a wrapped provider serves its functions. The schema lists them,
// GetFunctions answers, and CallFunction calls one — so an in-process provider
// served as a plugin (the built-in terraform provider's encode_tfvars,
// decode_tfvars and encode_expr) is callable as provider::<name>::<function>
// by a host that talks to it over tfplugin5, rather than panicking the
// server.

import (
	"context"
	"errors"
	"fmt"

	"github.com/opentofu/opentofu/internal/plugin/convert"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfplugin5"
)

// functionsToProto is a provider's function specifications as the schema and
// GetFunctions carry them.
func functionsToProto(specs map[string]providers.FunctionSpec) map[string]*tfplugin5.Function {
	if len(specs) == 0 {
		return nil
	}
	out := make(map[string]*tfplugin5.Function, len(specs))
	for name, spec := range specs {
		out[name] = convert.FunctionSpecToProto(spec)
	}
	return out
}

func (p *provider) GetFunctions(ctx context.Context, _ *tfplugin5.GetFunctions_Request) (*tfplugin5.GetFunctions_Response, error) {
	resp := &tfplugin5.GetFunctions_Response{}
	funcs := p.provider.GetFunctions(ctx)
	resp.Diagnostics = convert.AppendProtoDiag(resp.Diagnostics, funcs.Diagnostics)
	if funcs.Diagnostics.HasErrors() {
		return resp, nil
	}
	resp.Functions = functionsToProto(funcs.Functions)
	return resp, nil
}

// functionSpec is the named function's specification: the schema's, else the
// provider's GetFunctions, as the client half looks one up.
func (p *provider) functionSpec(ctx context.Context, name string) (providers.FunctionSpec, error) {
	if spec, ok := p.schema.Functions[name]; ok {
		return spec, nil
	}
	funcs := p.provider.GetFunctions(ctx)
	if funcs.Diagnostics.HasErrors() {
		return providers.FunctionSpec{}, funcs.Diagnostics.Err()
	}
	if spec, ok := funcs.Functions[name]; ok {
		return spec, nil
	}
	return providers.FunctionSpec{}, fmt.Errorf("provider function %q not found", name)
}

func (p *provider) CallFunction(ctx context.Context, req *tfplugin5.CallFunction_Request) (*tfplugin5.CallFunction_Response, error) {
	resp := &tfplugin5.CallFunction_Response{}

	spec, err := p.functionSpec(ctx, req.Name)
	if err != nil {
		resp.Error = &tfplugin5.FunctionError{Text: err.Error()}
		return resp, nil
	}
	if len(req.Arguments) < len(spec.Parameters) || (spec.VariadicParameter == nil && len(req.Arguments) > len(spec.Parameters)) {
		resp.Error = &tfplugin5.FunctionError{
			Text: fmt.Sprintf("function %s takes %d arguments, but was called with %d", req.Name, len(spec.Parameters), len(req.Arguments)),
		}
		return resp, nil
	}

	// Each argument decodes with its parameter's type, as the client encoded
	// it: a dynamic parameter's value carries its own type, and an absent one
	// is the null the client leaves out.
	callReq := providers.CallFunctionRequest{Name: req.Name}
	for i, arg := range req.Arguments {
		param := spec.VariadicParameter
		if i < len(spec.Parameters) {
			param = &spec.Parameters[i]
		}
		val, err := decodeDynamicValue(arg, param.Type)
		if err != nil {
			resp.Error = functionArgumentError(i, err.Error())
			return resp, nil
		}
		callReq.Arguments = append(callReq.Arguments, val)
	}

	callResp := p.provider.CallFunction(ctx, callReq)
	if callResp.Error != nil {
		// An argument error keeps the argument it blames; any other error
		// goes without one, and the client decides whom to blame.
		var argErr *providers.CallFunctionArgumentError
		if errors.As(callResp.Error, &argErr) {
			resp.Error = functionArgumentError(argErr.FunctionArgument, argErr.Text)
		} else {
			resp.Error = &tfplugin5.FunctionError{Text: callResp.Error.Error()}
		}
		return resp, nil
	}

	result, err := encodeDynamicValue(callResp.Result, spec.Return)
	if err != nil {
		resp.Error = &tfplugin5.FunctionError{Text: err.Error()}
		return resp, nil
	}
	resp.Result = result
	return resp, nil
}

func functionArgumentError(arg int, text string) *tfplugin5.FunctionError {
	index := int64(arg)
	return &tfplugin5.FunctionError{Text: text, FunctionArgument: &index}
}
