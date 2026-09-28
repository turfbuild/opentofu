// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// ExtractedReference represents a reference found in an HCL expression.
type ExtractedReference struct {
	// Subject is the address being referenced (e.g., "aws_instance.web", "var.region")
	Subject string

	// Type classifies the reference (resource, data, variable, local, module, etc.)
	Type ReferenceType

	// Ref is the typed subject Subject and Type are rendered from — OpenTofu's
	// own addrs.Referenceable, exposed through the x/addrs aliases. Consumers
	// that need the reference's structure (the resource address, an instance
	// key, a module call's output name) should read it here rather than
	// re-parsing Subject.
	Ref addrs.Referenceable
}

// ReferenceType classifies the type of reference.
type ReferenceType string

const (
	ReferenceTypeResource  ReferenceType = "resource"
	ReferenceTypeData      ReferenceType = "data"
	ReferenceTypeEphemeral ReferenceType = "ephemeral"
	ReferenceTypeVariable  ReferenceType = "variable"
	ReferenceTypeLocal     ReferenceType = "local"
	ReferenceTypeModule    ReferenceType = "module"
	ReferenceTypeOutput    ReferenceType = "output"
	ReferenceTypePath      ReferenceType = "path"
	ReferenceTypeTerraform ReferenceType = "terraform"
	ReferenceTypeSelf      ReferenceType = "self"
	ReferenceTypeCount     ReferenceType = "count"
	ReferenceTypeEach      ReferenceType = "each"
	ReferenceTypeUnknown   ReferenceType = "unknown"
)

// ExtractReferences extracts all references from an HCL expression.
func ExtractReferences(expr hcl.Expression) ([]ExtractedReference, error) {
	return extractReferencesExcluding(expr, nil)
}

// extractReferencesExcluding is ExtractReferences with a set of root names
// that are not references at all: the iterators of the `dynamic` blocks an
// expression sits inside. `setting.value` in a dynamic block's content is the
// iterator, not a resource named `setting` — and OpenTofu's ParseRef reads any
// root it does not know as a managed resource, so the name has to be dropped
// before it is parsed. References skips a nil reference.
func extractReferencesExcluding(expr hcl.Expression, iterators map[string]struct{}) ([]ExtractedReference, error) {
	parseRef := addrs.ParseRef
	if len(iterators) > 0 {
		parseRef = func(traversal hcl.Traversal) (*addrs.Reference, tfdiags.Diagnostics) {
			if _, ok := iterators[traversal.RootName()]; ok {
				return nil, nil
			}
			return addrs.ParseRef(traversal)
		}
	}
	refs, diags := lang.ReferencesInExpr(parseRef, expr)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to extract references: %s", diags.Err())
	}

	result := make([]ExtractedReference, 0, len(refs))
	for _, ref := range refs {
		result = append(result, ExtractedReference{
			Subject: formatSubject(ref.Subject),
			Type:    classifyReference(ref.Subject),
			Ref:     ref.Subject,
		})
	}
	return result, nil
}

// classifyReference determines the type of a reference based on its subject.
func classifyReference(subject addrs.Referenceable) ReferenceType {
	switch s := subject.(type) {
	case addrs.Resource:
		return classifyResourceMode(s.Mode)
	case addrs.ResourceInstance:
		return classifyResourceMode(s.Resource.Mode)
	case addrs.InputVariable:
		return ReferenceTypeVariable
	case addrs.LocalValue:
		return ReferenceTypeLocal
	case addrs.ModuleCall:
		return ReferenceTypeModule
	case addrs.ModuleCallInstance:
		return ReferenceTypeModule
	case addrs.ModuleCallInstanceOutput:
		return ReferenceTypeModule
	case addrs.ModuleCallOutput:
		return ReferenceTypeModule
	case addrs.OutputValue:
		return ReferenceTypeOutput
	case addrs.PathAttr:
		return ReferenceTypePath
	case addrs.TerraformAttr:
		return ReferenceTypeTerraform
	case addrs.CountAttr:
		return ReferenceTypeCount
	case addrs.ForEachAttr:
		return ReferenceTypeEach
	default:
		return ReferenceTypeUnknown
	}
}

// classifyResourceMode maps a resource mode onto its reference type. The three
// modes are distinct kinds of object, not variations on one: they are declared
// by different block types, and a consumer keying dependency targets off a
// reference has to be able to tell them apart.
func classifyResourceMode(mode addrs.ResourceMode) ReferenceType {
	switch mode {
	case addrs.DataResourceMode:
		return ReferenceTypeData
	case addrs.EphemeralResourceMode:
		return ReferenceTypeEphemeral
	default:
		return ReferenceTypeResource
	}
}

// formatSubject converts a Referenceable to a human-readable string.
func formatSubject(subject addrs.Referenceable) string {
	switch s := subject.(type) {
	case addrs.Resource:
		// Resource.String() already renders each mode's prefix — bare for
		// managed, `data.` for data, `ephemeral.` for ephemeral. Rendering the
		// prefix here by hand is what let ephemeral fall through as bare and
		// collide with a managed resource of the same type and name.
		return s.String()
	case addrs.ResourceInstance:
		base := formatSubject(s.Resource)
		if s.Key != addrs.NoKey {
			return fmt.Sprintf("%s%s", base, s.Key)
		}
		return base
	case addrs.InputVariable:
		return fmt.Sprintf("var.%s", s.Name)
	case addrs.LocalValue:
		return fmt.Sprintf("local.%s", s.Name)
	case addrs.ModuleCall:
		return fmt.Sprintf("module.%s", s.Name)
	case addrs.ModuleCallInstance:
		return s.String()
	case addrs.ModuleCallInstanceOutput:
		return s.String()
	case addrs.ModuleCallOutput:
		return fmt.Sprintf("module.%s.%s", s.Call.Name, s.Name)
	case addrs.OutputValue:
		return fmt.Sprintf("output.%s", s.Name)
	case addrs.PathAttr:
		return fmt.Sprintf("path.%s", s.Name)
	case addrs.TerraformAttr:
		return fmt.Sprintf("terraform.%s", s.Name)
	case addrs.CountAttr:
		return fmt.Sprintf("count.%s", s.Name)
	case addrs.ForEachAttr:
		return fmt.Sprintf("each.%s", s.Name)
	default:
		return subject.String()
	}
}

// ExtractReferencesFromBody extracts all references from an HCL body.
// This walks both attributes and nested blocks recursively to find all references.
func ExtractReferencesFromBody(body hcl.Body) ([]ExtractedReference, error) {
	if body == nil {
		return nil, nil
	}

	syntaxBody, ok := body.(*hclsyntax.Body)
	if !ok {
		// Fall back to JustAttributes for non-syntax bodies
		attrs, _ := body.JustAttributes()
		if attrs == nil {
			return nil, nil
		}
		var allRefs []ExtractedReference
		for _, attr := range attrs {
			refs, err := ExtractReferences(attr.Expr)
			if err != nil {
				continue
			}
			allRefs = append(allRefs, refs...)
		}
		return allRefs, nil
	}

	return extractRefsFromSyntaxBody(syntaxBody, nil), nil
}

// extractRefsFromSyntaxBody recursively extracts references from an hclsyntax.Body,
// walking both attributes and nested blocks.
//
// It walks what the body still *carries as configuration* — see visible.go. A
// meta-argument a decode has already lifted into its own field is not
// configuration: `provider = aws.west` is a provider reference, and reading it
// here would report a reference to a resource named `aws.west` that nobody
// declared. Whoever consumed it owns the references it holds.
//
// iterators holds the names of the `dynamic` block iterators in scope, which
// are not references; see extractRefsFromDynamicBlock.
func extractRefsFromSyntaxBody(body *hclsyntax.Body, iterators map[string]struct{}) []ExtractedReference {
	if body == nil {
		return nil
	}

	var allRefs []ExtractedReference

	for _, attr := range visibleAttributes(body) {
		refs, err := extractReferencesExcluding(attr.Expr, iterators)
		if err != nil {
			continue
		}
		allRefs = append(allRefs, refs...)
	}

	// Nested blocks recursively: a reference can sit any number of blocks down
	// (a provider's `resource {}` shape, a nested `metadata {}`), which is why
	// the whole body is walked rather than its top-level attributes.
	for _, block := range visibleBlocks(body) {
		if block.Type == "dynamic" {
			allRefs = append(allRefs, extractRefsFromDynamicBlock(block, iterators)...)
			continue
		}
		allRefs = append(allRefs, extractRefsFromSyntaxBody(block.Body, iterators)...)
	}

	return allRefs
}

// extractRefsFromDynamicBlock walks a `dynamic` block the way HCL's dynblock
// reads it (ext/dynblock's WalkVariablesNode.Visit), with no schema: the walk
// runs before any provider is loaded. A dynamic block is a generator, not
// configuration — its body is for_each, iterator, labels and one content
// block — so it cannot be walked as a plain nested block, where the
// iterator's own name reads as a reference to an undeclared resource.
//
// The iterator is named by the `iterator` argument, or else by the block's
// label. for_each is evaluated outside the iteration, so it sees only the
// iterators it inherits; labels and content see this block's too. An
// `iterator` that is not a bare name makes the block invalid, and the block is
// skipped, as dynblock skips it: the decode reports it. So is a block with
// other than one label.
func extractRefsFromDynamicBlock(block *hclsyntax.Block, inherited map[string]struct{}) []ExtractedReference {
	if len(block.Labels) != 1 {
		return nil
	}
	// A dynamic block's own body is never a decode's remain, so its maps are
	// read directly.
	name := block.Labels[0]
	if attr, ok := block.Body.Attributes["iterator"]; ok {
		traversal, _ := hcl.AbsTraversalForExpr(attr.Expr)
		if len(traversal) == 0 {
			return nil
		}
		name = traversal.RootName()
	}
	own := make(map[string]struct{}, len(inherited)+1)
	for n := range inherited {
		own[n] = struct{}{}
	}
	own[name] = struct{}{}

	var allRefs []ExtractedReference
	if attr, ok := block.Body.Attributes["for_each"]; ok {
		if refs, err := extractReferencesExcluding(attr.Expr, inherited); err == nil {
			allRefs = append(allRefs, refs...)
		}
	}
	if attr, ok := block.Body.Attributes["labels"]; ok {
		if refs, err := extractReferencesExcluding(attr.Expr, own); err == nil {
			allRefs = append(allRefs, refs...)
		}
	}
	for _, content := range block.Body.Blocks {
		if content.Type == "content" {
			allRefs = append(allRefs, extractRefsFromSyntaxBody(content.Body, own)...)
		}
	}
	return allRefs
}

// FormatTraversal converts an HCL traversal to a string representation.
// For example, a traversal of [google_compute_network, vpc, id] becomes "google_compute_network.vpc.id".
func FormatTraversal(traversal hcl.Traversal) string {
	var parts []string
	for _, step := range traversal {
		switch t := step.(type) {
		case hcl.TraverseRoot:
			parts = append(parts, t.Name)
		case hcl.TraverseAttr:
			parts = append(parts, t.Name)
		case hcl.TraverseIndex:
			// Handle index expressions like [0] or ["key"]
			if t.Key.Type() == cty.Number {
				idx, _ := t.Key.AsBigFloat().Int64()
				if len(parts) > 0 {
					parts[len(parts)-1] += fmt.Sprintf("[%d]", idx)
				}
			} else if t.Key.Type() == cty.String {
				if len(parts) > 0 {
					parts[len(parts)-1] += fmt.Sprintf("[%q]", t.Key.AsString())
				}
			}
		}
	}
	return strings.Join(parts, ".")
}
