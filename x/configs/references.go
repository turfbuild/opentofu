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
	refs, diags := lang.ReferencesInExpr(addrs.ParseRef, expr)
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

// ResourceReferences extracts only resource references (not variables, locals, etc.).
func ResourceReferences(refs []ExtractedReference) []string {
	var result []string
	for _, ref := range refs {
		if ref.Type == ReferenceTypeResource || ref.Type == ReferenceTypeData || ref.Type == ReferenceTypeEphemeral {
			result = append(result, ref.Subject)
		}
	}
	return result
}

// UniqueReferences returns unique reference subjects from a list.
func UniqueReferences(refs []ExtractedReference) []string {
	seen := make(map[string]bool)
	var result []string
	for _, ref := range refs {
		if !seen[ref.Subject] {
			seen[ref.Subject] = true
			result = append(result, ref.Subject)
		}
	}
	return result
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

	return extractRefsFromSyntaxBody(syntaxBody), nil
}

// extractRefsFromSyntaxBody recursively extracts references from an hclsyntax.Body,
// walking both attributes and nested blocks.
//
// It walks what the body still *carries as configuration* — see visible.go. A
// meta-argument a decode has already lifted into its own field is not
// configuration: `provider = aws.west` is a provider reference, and reading it
// here would report a reference to a resource named `aws.west` that nobody
// declared. Whoever consumed it owns the references it holds.
func extractRefsFromSyntaxBody(body *hclsyntax.Body) []ExtractedReference {
	if body == nil {
		return nil
	}

	var allRefs []ExtractedReference

	for _, attr := range visibleAttributes(body) {
		refs, err := ExtractReferences(attr.Expr)
		if err != nil {
			continue
		}
		allRefs = append(allRefs, refs...)
	}

	// Nested blocks recursively: a reference can sit any number of blocks down
	// (a provider's `resource {}` shape, a nested `metadata {}`), which is why
	// the whole body is walked rather than its top-level attributes.
	for _, block := range visibleBlocks(body) {
		blockRefs := extractRefsFromSyntaxBody(block.Body)
		allRefs = append(allRefs, blockRefs...)
	}

	return allRefs
}

// FormatExpressionTraversal returns the full traversal path of an expression.
// For "google_compute_network.vpc.id", returns "google_compute_network.vpc.id".
// For complex expressions, falls back to the first reference subject.
func FormatExpressionTraversal(expr hcl.Expression) (string, error) {
	if expr == nil {
		return "", fmt.Errorf("nil expression")
	}

	// Try to get as a simple scope traversal (handles expressions like "resource.name.attr")
	traversal, diags := hcl.AbsTraversalForExpr(expr)
	if !diags.HasErrors() && len(traversal) > 0 {
		return FormatTraversal(traversal), nil
	}

	// Fall back to extracting references
	refs, err := ExtractReferences(expr)
	if err != nil || len(refs) == 0 {
		return "", fmt.Errorf("cannot format expression: no references found")
	}
	return refs[0].Subject, nil
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
