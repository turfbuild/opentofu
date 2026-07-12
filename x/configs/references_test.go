// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"sort"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

func TestFormatExpressionTraversal(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected string
		wantErr  bool
	}{
		{
			name:     "simple resource reference with attribute",
			expr:     "google_compute_network.vpc.id",
			expected: "google_compute_network.vpc.id",
		},
		{
			name:     "resource reference with name attribute",
			expr:     "aws_instance.web.public_ip",
			expected: "aws_instance.web.public_ip",
		},
		{
			name:     "variable reference",
			expr:     "var.region",
			expected: "var.region",
		},
		{
			name:     "local reference",
			expr:     "local.cluster_name",
			expected: "local.cluster_name",
		},
		{
			name:     "data source reference",
			expr:     "data.aws_ami.latest.id",
			expected: "data.aws_ami.latest.id",
		},
		{
			name:     "resource without attribute (whole object)",
			expr:     "google_compute_network.vpc",
			expected: "google_compute_network.vpc",
		},
		{
			name:     "indexed resource",
			expr:     "aws_instance.web[0].id",
			expected: "aws_instance.web[0].id",
		},
		{
			name:     "resource with string index",
			expr:     `aws_instance.web["primary"].id`,
			expected: `aws_instance.web["primary"].id`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse the expression
			expr, diags := hclsyntax.ParseExpression([]byte(tt.expr), "", hcl.InitialPos)
			if diags.HasErrors() {
				t.Fatalf("failed to parse expression %q: %s", tt.expr, diags.Error())
			}

			got, err := FormatExpressionTraversal(expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("FormatExpressionTraversal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("FormatExpressionTraversal() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func parseHCLBody(t *testing.T, src string) hcl.Body {
	t.Helper()
	file, diags := hclsyntax.ParseConfig([]byte(src), "test.tf", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("parse error: %s", diags.Error())
	}
	return file.Body
}

func TestExtractReferencesFromBody_TopLevelAttributes(t *testing.T) {
	body := parseHCLBody(t, `
		vpc_id    = aws_vpc.main.id
		subnet_id = aws_subnet.public.id
		name      = "literal"
	`)

	refs, err := ExtractReferencesFromBody(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	unique := UniqueReferences(refs)
	sort.Strings(unique)

	// References are extracted at the subject level (type.name), not attribute level
	expected := []string{"aws_subnet.public", "aws_vpc.main"}
	if len(unique) != len(expected) {
		t.Fatalf("got %v, want %v", unique, expected)
	}
	for i, e := range expected {
		if unique[i] != e {
			t.Errorf("unique[%d] = %q, want %q", i, unique[i], e)
		}
	}
}

func TestExtractReferencesFromBody_NestedBlocks(t *testing.T) {
	// Simulate a Kubernetes-style resource with nested metadata block
	body := parseHCLBody(t, `
		name = kubernetes_namespace.example.metadata[0].name
		metadata {
			labels = var.labels
			nested {
				ref = data.aws_ami.latest.id
			}
		}
	`)

	refs, err := ExtractReferencesFromBody(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	unique := UniqueReferences(refs)
	sort.Strings(unique)

	// Should find references from top-level attrs AND nested blocks
	// References are at subject level: type.name or data.type.name or var.name
	if len(unique) < 3 {
		t.Fatalf("expected at least 3 references, got %v", unique)
	}

	// Check that references from nested blocks are found
	found := make(map[string]bool)
	for _, r := range unique {
		found[r] = true
	}

	if !found["var.labels"] {
		t.Errorf("missing reference: var.labels (from nested metadata block), got %v", unique)
	}
	if !found["data.aws_ami.latest"] {
		t.Errorf("missing reference: data.aws_ami.latest (from doubly-nested block), got %v", unique)
	}
	if !found["kubernetes_namespace.example"] {
		t.Errorf("missing reference: kubernetes_namespace.example (from top-level attr), got %v", unique)
	}
}

func TestExtractReferencesFromBody_NilBody(t *testing.T) {
	refs, err := ExtractReferencesFromBody(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("expected no refs from nil body, got %v", refs)
	}
}

func TestFormatTraversal_WithIndex(t *testing.T) {
	// Parse "aws_instance.web[0].id" as a traversal
	expr, diags := hclsyntax.ParseExpression([]byte("aws_instance.web[0].id"), "", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("parse error: %s", diags.Error())
	}

	traversal, diags := hcl.AbsTraversalForExpr(expr)
	if diags.HasErrors() {
		t.Fatalf("not a traversal: %s", diags.Error())
	}

	result := FormatTraversal(traversal)
	expected := "aws_instance.web[0].id"
	if result != expected {
		t.Errorf("FormatTraversal() = %q, want %q", result, expected)
	}
}
