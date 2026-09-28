// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/opentofu/opentofu/internal/addrs"
)

// TestCaller pins that a parsed "caller" reference compares equal to the
// facade's Caller, so a consumer can recognize the subject by identity rather
// than by its rendered form.
func TestCaller(t *testing.T) {
	trav, diags := hclsyntax.ParseTraversalAbs([]byte("caller.id"), "", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	ref, refDiags := addrs.ParseRef(trav)
	if refDiags.HasErrors() {
		t.Fatal(refDiags.Err())
	}
	if ref.Subject != Caller {
		t.Fatalf("subject %#v is not Caller", ref.Subject)
	}
	var subject Referenceable = Caller
	if subject.String() != "caller" {
		t.Errorf("Caller renders as %q", subject.String())
	}
}
