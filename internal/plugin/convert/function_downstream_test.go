// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package convert

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/zclconf/go-cty/cty"
)

// TestFunctionSpecRoundTrip pins that the server half inverts the client half:
// a specification survives FunctionSpecToProto then ProtoToFunctionSpec, a
// dynamic parameter and return, a variadic tail and both text formats
// included.
func TestFunctionSpecRoundTrip(t *testing.T) {
	for name, spec := range map[string]providers.FunctionSpec{
		"plain": {
			Parameters: []providers.FunctionParameterSpec{{
				Name:              "content",
				Type:              cty.String,
				Description:       "content to decode",
				DescriptionFormat: providers.TextFormattingPlain,
			}},
			Return:            cty.DynamicPseudoType,
			Summary:           "decode",
			Description:       "decodes",
			DescriptionFormat: providers.TextFormattingPlain,
		},
		"variadic and markdown": {
			Parameters: []providers.FunctionParameterSpec{{
				Name:               "input",
				Type:               cty.DynamicPseudoType,
				AllowNullValue:     true,
				AllowUnknownValues: true,
				DescriptionFormat:  providers.TextFormattingMarkdown,
			}},
			VariadicParameter: &providers.FunctionParameterSpec{
				Name:              "rest",
				Type:              cty.List(cty.Object(map[string]cty.Type{"a": cty.Number})),
				DescriptionFormat: providers.TextFormattingPlain,
			},
			Return:             cty.Map(cty.String),
			DescriptionFormat:  providers.TextFormattingMarkdown,
			DeprecationMessage: "use another",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := ProtoToFunctionSpec(FunctionSpecToProto(spec))
			if diff := cmp.Diff(spec, got, cmp.Comparer(cty.Type.Equals)); diff != "" {
				t.Error(diff)
			}
		})
	}
}

// An unset format is sent as plain text, which is how the client reads an
// unset StringKind back.
func TestTextFormattingToProtoUnset(t *testing.T) {
	if got := ProtoToTextFormatting(TextFormattingToProto("")); got != providers.TextFormattingPlain {
		t.Errorf("unset format round-trips as %q", got)
	}
}
