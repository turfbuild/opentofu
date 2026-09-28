// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"sort"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// A decoded block hands back a *remain* body: `PartialContent` returns a Body
// carrying the original, complete Attributes and Blocks maps plus a private
// record of which of them it consumed. So walking those maps directly reads the
// whole block again — meta-arguments included — and a `provider = aws.west`
// comes back looking exactly like a reference to a resource named `aws.west`.
//
// hclsyntax keeps that record unexported, so a body cannot be asked what was
// taken from it. Two accessors know: JustAttributes and PartialContent. The
// helpers here are the only way this package should reach a body's contents.

// visibleAttributes returns the attributes no decode has consumed, in name
// order. Sorted because the underlying map iteration is not, and these feed
// dependency edges that end up persisted in state.
func visibleAttributes(body *hclsyntax.Body) []*hclsyntax.Attribute {
	if body == nil || len(body.Attributes) == 0 {
		return nil
	}
	// JustAttributes reports every block as unexpected; that diagnostic is about
	// a shape this function does not care about, and it returns the attributes
	// regardless.
	attrs, _ := body.JustAttributes()

	out := make([]*hclsyntax.Attribute, 0, len(attrs))
	for name := range attrs {
		if attr, ok := body.Attributes[name]; ok {
			out = append(out, attr)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// visibleBlocks returns the blocks no decode has consumed, in source order.
//
// PartialContent skips a consumed block type before it looks at anything else,
// so asking for every type the body actually contains and seeing which come
// back is a direct read of the record hclsyntax will not expose. Types are
// grouped by label arity because a header schema whose label count disagrees
// with the block's is reported as malformed and skipped, which would read as
// "consumed".
func visibleBlocks(body *hclsyntax.Body) []*hclsyntax.Block {
	if body == nil || len(body.Blocks) == 0 {
		return nil
	}

	typesByArity := make(map[int]map[string]struct{})
	for _, block := range body.Blocks {
		arity := len(block.Labels)
		if typesByArity[arity] == nil {
			typesByArity[arity] = make(map[string]struct{})
		}
		typesByArity[arity][block.Type] = struct{}{}
	}

	visible := make(map[string]struct{}, len(body.Blocks))
	for arity, types := range typesByArity {
		schema := &hcl.BodySchema{Blocks: make([]hcl.BlockHeaderSchema, 0, len(types))}
		for typeName := range types {
			schema.Blocks = append(schema.Blocks, hcl.BlockHeaderSchema{
				Type:       typeName,
				LabelNames: make([]string, arity),
			})
		}
		content, _, _ := body.PartialContent(schema)
		for _, block := range content.Blocks {
			visible[block.Type] = struct{}{}
		}
	}

	out := make([]*hclsyntax.Block, 0, len(body.Blocks))
	for _, block := range body.Blocks {
		if _, ok := visible[block.Type]; ok {
			out = append(out, block)
		}
	}
	return out
}
