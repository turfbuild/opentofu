// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package convert

// Downstream: the server half of function.go — a provider function's
// specification as the protocol carries it — for grpcwrap, which serves an
// in-process provider's functions to a client over tfplugin5.

import (
	"encoding/json"

	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfplugin5"
	"github.com/zclconf/go-cty/cty"
)

// CtyTypeToProto is ProtoToCtyType's inverse: a type as its JSON encoding.
func CtyTypeToProto(ty cty.Type) []byte {
	out, err := json.Marshal(ty)
	if err != nil {
		// Only a capsule type fails to marshal, and no function signature
		// can carry one across the protocol in either direction.
		panic(err)
	}
	return out
}

// TextFormattingToProto is ProtoToTextFormatting's inverse. An unset format is
// plain text, as an unset StringKind is.
func TextFormattingToProto(format providers.TextFormatting) tfplugin5.StringKind {
	if format == providers.TextFormattingMarkdown {
		return tfplugin5.StringKind_MARKDOWN
	}
	return tfplugin5.StringKind_PLAIN
}

// FunctionParameterSpecToProto is ProtoToFunctionParameterSpec's inverse.
func FunctionParameterSpecToProto(spec providers.FunctionParameterSpec) *tfplugin5.Function_Parameter {
	return &tfplugin5.Function_Parameter{
		Name:               spec.Name,
		Type:               CtyTypeToProto(spec.Type),
		AllowNullValue:     spec.AllowNullValue,
		AllowUnknownValues: spec.AllowUnknownValues,
		Description:        spec.Description,
		DescriptionKind:    TextFormattingToProto(spec.DescriptionFormat),
	}
}

// FunctionSpecToProto is ProtoToFunctionSpec's inverse.
func FunctionSpecToProto(spec providers.FunctionSpec) *tfplugin5.Function {
	params := make([]*tfplugin5.Function_Parameter, len(spec.Parameters))
	for i, param := range spec.Parameters {
		params[i] = FunctionParameterSpecToProto(param)
	}

	var varParam *tfplugin5.Function_Parameter
	if spec.VariadicParameter != nil {
		varParam = FunctionParameterSpecToProto(*spec.VariadicParameter)
	}

	return &tfplugin5.Function{
		Parameters:         params,
		VariadicParameter:  varParam,
		Return:             &tfplugin5.Function_Return{Type: CtyTypeToProto(spec.Return)},
		Summary:            spec.Summary,
		Description:        spec.Description,
		DescriptionKind:    TextFormattingToProto(spec.DescriptionFormat),
		DeprecationMessage: spec.DeprecationMessage,
	}
}
