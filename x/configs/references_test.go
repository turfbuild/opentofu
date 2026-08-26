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

// A decoded block hands back a body that still carries every attribute and
// block the source had, plus a private note of which ones it took. Reading the
// maps directly reads the whole block again — so `provider = aws.west`, an
// address the decode already lifted into its own field, comes back looking like
// a reference to a resource named `aws.west` that nobody declared.
func TestExtractReferencesFromBodyIgnoresWhatTheDecodeConsumed(t *testing.T) {
	body := parseHCLBody(t, `
		provider     = aws.west
		count        = length(aws_subnet.counted)
		depends_on   = [aws_vpc.declared]
		ami          = aws_ami.chosen.id
		lifecycle {
		  replace_triggered_by = [aws_instance.trigger]
		}
		tags {
		  owner = aws_iam_user.owner.name
		}
	`)

	// What a resource decode consumes; see configs.ResourceBlockSchema.
	_, remain, diags := body.PartialContent(&hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "count"}, {Name: "for_each"}, {Name: "provider"}, {Name: "depends_on"},
		},
		Blocks: []hcl.BlockHeaderSchema{{Type: "lifecycle"}},
	})
	if diags.HasErrors() {
		t.Fatalf("partial content: %s", diags.Error())
	}

	refs, err := ExtractReferencesFromBody(remain)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := UniqueReferences(refs)
	sort.Strings(got)

	// The `tags` block survives: nothing consumed it, and a reference sitting a
	// block down is the whole reason the walk recurses.
	want := []string{"aws_ami.chosen", "aws_iam_user.owner"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// The same body, undecoded, is all configuration — nothing has been consumed,
// so nothing is withheld. This is what keeps the fix from being a filter on
// attribute names: an action's `config {}` block may legitimately carry an
// attribute called `provider` or `count`.
func TestExtractReferencesFromBodyKeepsEverythingWhenNothingWasConsumed(t *testing.T) {
	body := parseHCLBody(t, `
		provider = aws.west
		ami      = aws_ami.chosen.id
	`)

	refs, err := ExtractReferencesFromBody(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := UniqueReferences(refs)
	sort.Strings(got)

	want := []string{"aws.west", "aws_ami.chosen"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// An ephemeral reference must render with its `ephemeral.` prefix, exactly as
// a data reference renders with `data.`. Consumers key graph dependency
// targets off Subject, so an ephemeral rendered bare would be indistinguishable
// from a managed resource of the same type and name — two different objects
// collapsing onto one node.
func TestExtractReferencesDistinguishesEphemeralFromManaged(t *testing.T) {
	tests := []struct {
		name        string
		expr        string
		wantSubject string
		wantType    ReferenceType
	}{
		{
			name:        "ephemeral whole object",
			expr:        "ephemeral.vault_kv_secret.creds",
			wantSubject: "ephemeral.vault_kv_secret.creds",
			wantType:    ReferenceTypeEphemeral,
		},
		{
			name:        "ephemeral with attribute",
			expr:        "ephemeral.vault_kv_secret.creds.token",
			wantSubject: "ephemeral.vault_kv_secret.creds",
			wantType:    ReferenceTypeEphemeral,
		},
		{
			name:        "ephemeral with instance key",
			expr:        `ephemeral.vault_kv_secret.creds["a"].token`,
			wantSubject: `ephemeral.vault_kv_secret.creds["a"]`,
			wantType:    ReferenceTypeEphemeral,
		},
		{
			name:        "managed resource of the same type and name",
			expr:        "vault_kv_secret.creds.token",
			wantSubject: "vault_kv_secret.creds",
			wantType:    ReferenceTypeResource,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, diags := hclsyntax.ParseExpression([]byte(tt.expr), "test.tf", hcl.Pos{Line: 1, Column: 1})
			if diags.HasErrors() {
				t.Fatalf("failed to parse expression: %s", diags.Error())
			}
			refs, err := ExtractReferences(expr)
			if err != nil {
				t.Fatalf("ExtractReferences() error = %v", err)
			}
			if len(refs) != 1 {
				t.Fatalf("got %d references, want 1: %#v", len(refs), refs)
			}
			if refs[0].Subject != tt.wantSubject {
				t.Errorf("Subject = %q, want %q", refs[0].Subject, tt.wantSubject)
			}
			if refs[0].Type != tt.wantType {
				t.Errorf("Type = %q, want %q", refs[0].Type, tt.wantType)
			}
		})
	}
}

// ResourceReferences is how a consumer asks "what objects does this expression
// depend on"; an ephemeral resource is such an object.
func TestResourceReferencesIncludesEphemeral(t *testing.T) {
	expr, diags := hclsyntax.ParseExpression(
		[]byte("ephemeral.vault_kv_secret.creds.token"), "test.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatalf("failed to parse expression: %s", diags.Error())
	}
	refs, err := ExtractReferences(expr)
	if err != nil {
		t.Fatalf("ExtractReferences() error = %v", err)
	}
	got := ResourceReferences(refs)
	if len(got) != 1 || got[0] != "ephemeral.vault_kv_secret.creds" {
		t.Errorf("ResourceReferences() = %v, want [ephemeral.vault_kv_secret.creds]", got)
	}
}
