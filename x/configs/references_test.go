// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"sort"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

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

	unique := uniqueSubjects(refs)
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

	unique := uniqueSubjects(refs)
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

// A `dynamic` block's iterator is not a reference. Walked as a plain nested
// block, `setting.value` reads as a resource named `setting` that nobody
// declared; the walk follows dynblock's scoping instead: for_each sees only the
// iterators it inherits, labels and content see the block's own as well, the
// iterator is renamed by `iterator`, and a block whose `iterator` is not a bare
// name is skipped (the decode reports it).
func TestExtractReferencesFromBodyWalksDynamicBlocks(t *testing.T) {
	body := parseHCLBody(t, `
		name = var.name
		setting {
			string = aws_static.before.id
		}
		dynamic "setting" {
			for_each = var.settings
			content {
				value = setting.value
				key   = "${setting.key}-${local.suffix}"
				dynamic "inner" {
					for_each = setting.value.tags
					iterator = tag
					labels   = [tag.key, data.aws_label.l.name]
					content {
						t = tag.value
						s = setting.key
						r = aws_inner.x.id
					}
				}
			}
		}
		dynamic "aws_foreach" {
			for_each = aws_foreach.src.items
			content {
				v = aws_foreach.value
			}
		}
		dynamic "broken" {
			for_each = aws_skipped.nope.ids
			iterator = "not-a-name"
			content {
				x = broken.value
			}
		}
		setting {
			string = aws_static.after.id
		}
	`)

	refs, err := ExtractReferencesFromBody(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := uniqueSubjects(refs)
	sort.Strings(got)

	// aws_foreach.src is a reference: a block's own iterator is not in scope in
	// its for_each, so the name there is the resource it reads.
	want := []string{
		"aws_foreach.src", "aws_inner.x", "aws_static.after", "aws_static.before",
		"data.aws_label.l", "local.suffix", "var.name", "var.settings",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
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
	got := uniqueSubjects(refs)
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
	got := uniqueSubjects(refs)
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

// uniqueSubjects returns the distinct rendered subjects of refs, in first-seen
// order.
func uniqueSubjects(refs []ExtractedReference) []string {
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
