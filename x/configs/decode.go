// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/zclconf/go-cty/cty"
)

// ExprEvaluator evaluates an HCL expression and returns a resolved Go value.
// If the expression cannot be fully evaluated (e.g., references to unresolved resources),
// the evaluator may return a placeholder value or an error, depending on the context.
type ExprEvaluator func(expr hcl.Expression) (value any, err error)

// MarkedExprEvaluator evaluates an HCL expression and returns the resolved
// cty.Value with its marks intact (unlike ExprEvaluator, which flattens to a
// Go value and drops marks). Used to discover config-flow sensitivity — a
// reference to a schema-sensitive attribute lands here still marked. On a
// genuine evaluation error the caller should return (cty.NilVal, err); an
// unresolvable reference carries no mark, so skipping it loses nothing.
type MarkedExprEvaluator func(expr hcl.Expression) (value cty.Value, err error)

// hclSchemaFor derives the hcl.BodySchema that drives PartialContent from a
// provider configschema.Block: every attribute becomes an AttributeSchema and
// every nested block a BlockHeaderSchema. Only NestingMap blocks carry a label
// (the map key); the others are unlabeled. Building the schema this way lets the
// decoders below walk any hcl.Body — native (*hclsyntax.Body) or JSON
// (*json.body) — through the interface instead of concrete field access.
func hclSchemaFor(schema *configschema.Block) *hcl.BodySchema {
	s := &hcl.BodySchema{
		Attributes: make([]hcl.AttributeSchema, 0, len(schema.Attributes)),
		Blocks:     make([]hcl.BlockHeaderSchema, 0, len(schema.BlockTypes)),
	}
	for name := range schema.Attributes {
		s.Attributes = append(s.Attributes, hcl.AttributeSchema{Name: name})
	}
	for name, blockType := range schema.BlockTypes {
		var labels []string
		if blockType.Nesting == configschema.NestingMap {
			labels = []string{"key"}
		}
		s.Blocks = append(s.Blocks, hcl.BlockHeaderSchema{Type: name, LabelNames: labels})
	}
	return s
}

// DecodeBodySensitivePaths walks the same schema structure as DecodeBodyToConfig
// and returns, for each top-level attribute or block whose evaluated config
// carries a Sensitive mark anywhere beneath it, a path marking that whole
// top-level element. This recovers config-derived sensitivity (a sensitive
// value flowing into an attribute the provider schema does not itself mark)
// before it is lost at the Go boundary.
//
// Granularity is deliberately the top-level element, not the exact leaf: a
// `{...}` config literal evaluates to a cty object (GetAttrStep keys), but the
// provider returns the same attribute coerced to its schema type — a map or set
// (IndexStep / value-keyed) — so a leaf path built from the config structure
// would not match the returned value and MarkWithPaths would silently miss it,
// leaking the secret. A top-level element is always an object field of the
// resource value, so [GetAttr{name}] always matches; marking the whole element
// over-redacts a non-sensitive sibling but never under-redacts. The returned
// paths are Sensitive-only and suitable for cty.Value.MarkWithPaths.
func DecodeBodySensitivePaths(body hcl.Body, schema *configschema.Block, eval MarkedExprEvaluator) ([]cty.PathValueMarks, error) {
	if body == nil || schema == nil {
		return nil, nil
	}
	content, _, diags := body.PartialContent(hclSchemaFor(schema))
	if diags.HasErrors() {
		return nil, fmt.Errorf("decoding body: %s", diags.Error())
	}
	var out []cty.PathValueMarks
	for name := range schema.Attributes {
		attr, exists := content.Attributes[name]
		if !exists {
			continue
		}
		v, err := eval(attr.Expr)
		if err != nil || v == cty.NilVal {
			continue
		}
		if valueHasSensitive(v) {
			out = append(out, cty.PathValueMarks{
				Path:  cty.GetAttrPath(name),
				Marks: cty.NewValueMarks(marks.Sensitive),
			})
		}
	}
	for name, blockType := range schema.BlockTypes {
		sensitive := false
		for _, block := range content.Blocks.OfType(name) {
			if bodyHasSensitive(block.Body, &blockType.Block, eval) {
				sensitive = true
				break
			}
		}
		if sensitive {
			out = append(out, cty.PathValueMarks{
				Path:  cty.GetAttrPath(name),
				Marks: cty.NewValueMarks(marks.Sensitive),
			})
		}
	}
	return out, nil
}

// bodyHasSensitive reports whether any attribute in body (recursing into nested
// blocks) evaluates to a value carrying a Sensitive mark. A per-attribute
// evaluation error is treated as not sensitive: an unresolved reference has no
// mark, and a partial decode should still redact whatever it can resolve.
func bodyHasSensitive(body hcl.Body, schema *configschema.Block, eval MarkedExprEvaluator) bool {
	if body == nil || schema == nil {
		return false
	}
	content, _, diags := body.PartialContent(hclSchemaFor(schema))
	if diags.HasErrors() {
		return false
	}
	for name := range schema.Attributes {
		attr, exists := content.Attributes[name]
		if !exists {
			continue
		}
		v, err := eval(attr.Expr)
		if err != nil || v == cty.NilVal {
			continue
		}
		if valueHasSensitive(v) {
			return true
		}
	}
	for name, blockType := range schema.BlockTypes {
		for _, block := range content.Blocks.OfType(name) {
			if bodyHasSensitive(block.Body, &blockType.Block, eval) {
				return true
			}
		}
	}
	return false
}

// valueHasSensitive reports whether val carries the Sensitive mark anywhere.
func valueHasSensitive(val cty.Value) bool {
	_, pvms := val.UnmarkDeepWithPaths()
	for _, pvm := range pvms {
		if _, ok := pvm.Marks[marks.Sensitive]; ok {
			return true
		}
	}
	return false
}

// DecodeBodyToConfig decodes an HCL body into a map[string]any using a
// configschema.Block to determine the structure (which names are attributes vs
// blocks, nesting modes, etc.). The eval callback evaluates attribute
// expressions. The body may be native or JSON syntax — structure is resolved
// through the hcl.Body interface via PartialContent.
func DecodeBodyToConfig(body hcl.Body, schema *configschema.Block, eval ExprEvaluator) (map[string]any, error) {
	return decodeBody(body, schema, eval)
}

func decodeBody(body hcl.Body, schema *configschema.Block, eval ExprEvaluator) (map[string]any, error) {
	if body == nil || schema == nil {
		return nil, nil
	}

	content, _, diags := body.PartialContent(hclSchemaFor(schema))
	if diags.HasErrors() {
		return nil, fmt.Errorf("decoding body: %s", diags.Error())
	}

	result := make(map[string]any)

	// Process attributes defined in the schema
	for name := range schema.Attributes {
		attr, exists := content.Attributes[name]
		if !exists {
			continue
		}

		val, err := eval(attr.Expr)
		if err != nil {
			return nil, fmt.Errorf("evaluating attribute %q: %w", name, err)
		}

		if val != nil {
			result[name] = val
		}
	}

	// Process block types defined in the schema
	for name, blockType := range schema.BlockTypes {
		blocks := content.Blocks.OfType(name)
		if len(blocks) == 0 {
			continue
		}

		switch blockType.Nesting {
		case configschema.NestingSingle, configschema.NestingGroup:
			// Single block → map[string]any
			decoded, err := decodeBody(blocks[0].Body, &blockType.Block, eval)
			if err != nil {
				return nil, fmt.Errorf("decoding block %q: %w", name, err)
			}
			result[name] = decoded

		case configschema.NestingList, configschema.NestingSet:
			// List/set of blocks → []any{map, map, ...}
			var list []any
			for _, block := range blocks {
				decoded, err := decodeBody(block.Body, &blockType.Block, eval)
				if err != nil {
					return nil, fmt.Errorf("decoding block %q: %w", name, err)
				}
				list = append(list, decoded)
			}
			result[name] = list

		case configschema.NestingMap:
			// Map of blocks → map[string]any{label: map, ...}
			mapResult := make(map[string]any)
			for _, block := range blocks {
				if len(block.Labels) > 0 {
					decoded, err := decodeBody(block.Body, &blockType.Block, eval)
					if err != nil {
						return nil, fmt.Errorf("decoding block %q[%q]: %w", name, block.Labels[0], err)
					}
					mapResult[block.Labels[0]] = decoded
				}
			}
			result[name] = mapResult
		}
	}

	return result, nil
}

// ExtractBodyToConfig decodes an HCL body into a map[string]any WITHOUT a
// schema, preserving nested blocks. It is the schema-less counterpart to
// DecodeBodyToConfig, for callers that have a parsed body but no provider
// schema to decode against — notably config_init, which reads provider,
// backend, and required_providers blocks before any provider is loaded.
//
// Every attribute is evaluated via eval; every nested block is recursed into.
// Because there is no schema to say whether a block type is single, list, or
// map, the shape is inferred from the body: a block type appearing exactly once
// yields a single object (map), a block type appearing more than once yields a
// list of objects, and a block carrying a label yields a map keyed by label.
// A single unlabeled block therefore surfaces in object form — the shape an
// agent naturally writes; downstream schema-aware coercion (objchange
// LiftNestedBlocks) normalizes a single object into a MaxItems=1 block list
// where the real schema requires it.
//
// Only native-syntax bodies are supported (config_init parses .tf files); a
// JSON-syntax body returns an error rather than silently dropping blocks.
func ExtractBodyToConfig(body hcl.Body, eval ExprEvaluator) (map[string]any, error) {
	if body == nil {
		return nil, nil
	}
	syntaxBody, ok := body.(*hclsyntax.Body)
	if !ok {
		return nil, fmt.Errorf("expected *hclsyntax.Body, got %T", body)
	}
	return extractBody(syntaxBody, eval)
}

func extractBody(body *hclsyntax.Body, eval ExprEvaluator) (map[string]any, error) {
	if body == nil {
		return nil, nil
	}

	result := make(map[string]any)

	// Attributes.
	for name, attr := range body.Attributes {
		val, err := eval(attr.Expr)
		if err != nil {
			return nil, fmt.Errorf("evaluating attribute %q: %w", name, err)
		}
		if val != nil {
			result[name] = val
		}
	}

	// Blocks, grouped by type name (source order preserved within a type).
	seen := make(map[string]bool)
	for _, block := range body.Blocks {
		if seen[block.Type] {
			continue
		}
		seen[block.Type] = true

		blocks := blocksOfType(body, block.Type)
		labeled := false
		for _, b := range blocks {
			if len(b.Labels) > 0 {
				labeled = true
				break
			}
		}

		switch {
		case labeled:
			// Labeled blocks → map keyed by first label.
			m := make(map[string]any)
			for _, b := range blocks {
				if len(b.Labels) == 0 {
					continue
				}
				decoded, err := extractBody(b.Body, eval)
				if err != nil {
					return nil, fmt.Errorf("decoding block %q[%q]: %w", block.Type, b.Labels[0], err)
				}
				m[b.Labels[0]] = decoded
			}
			result[block.Type] = m
		case len(blocks) == 1:
			// Single unlabeled block → object.
			decoded, err := extractBody(blocks[0].Body, eval)
			if err != nil {
				return nil, fmt.Errorf("decoding block %q: %w", block.Type, err)
			}
			result[block.Type] = decoded
		default:
			// Repeated unlabeled blocks → list of objects.
			list := make([]any, 0, len(blocks))
			for _, b := range blocks {
				decoded, err := extractBody(b.Body, eval)
				if err != nil {
					return nil, fmt.Errorf("decoding block %q: %w", block.Type, err)
				}
				list = append(list, decoded)
			}
			result[block.Type] = list
		}
	}

	return result, nil
}

// blocksOfType returns all blocks in the body with the given type name, in
// source order.
func blocksOfType(body *hclsyntax.Body, typeName string) []*hclsyntax.Block {
	var result []*hclsyntax.Block
	for _, block := range body.Blocks {
		if block.Type == typeName {
			result = append(result, block)
		}
	}
	return result
}
