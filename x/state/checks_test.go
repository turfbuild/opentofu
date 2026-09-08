// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package state

import (
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/states/statefile"
)

// TestNewCheckResultsRoundTrips is the only assertion that matters for this
// shim: what goes in must come back out through OpenTofu's own statefile
// writer, unchanged. Anything else would be testing this file against itself.
func TestNewCheckResultsRoundTrips(t *testing.T) {
	in := []CheckResultInput{{
		ObjectKind: "var",
		ConfigAddr: "module.leaf.var.name",
		Status:     "pass",
		Objects: []CheckResultObjectInput{
			{ObjectAddr: `module.leaf[0].var.name`, Status: "pass"},
			{ObjectAddr: `module.leaf["dev"].var.name`, Status: "unknown"},
		},
	}, {
		ObjectKind: "var",
		ConfigAddr: "var.floor",
		Status:     "fail",
		Objects: []CheckResultObjectInput{
			{ObjectAddr: "var.floor", Status: "fail", FailureMessages: []string{"floor must be positive."}},
		},
	}}

	built, err := NewCheckResults(in)
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	st := NewState()
	st.CheckResults = built

	enc := encryption.StateEncryptionDisabled()
	var buf strings.Builder
	if err := statefile.Write(statefile.New(st, "lineage", 1), &buf, enc); err != nil {
		t.Fatalf("writing the statefile: %v", err)
	}
	back, err := statefile.Read(strings.NewReader(buf.String()), enc)
	if err != nil {
		t.Fatalf("reading it back: %v", err)
	}
	got := back.State.CheckResults
	if got == nil || got.ConfigResults.Len() != 2 {
		t.Fatalf("round-tripped %v configuration objects; want 2", got)
	}
	for _, want := range in {
		var found bool
		for _, elem := range got.ConfigResults.Elems {
			if elem.Key.String() != want.ConfigAddr {
				continue
			}
			found = true
			if elem.Value.ObjectResults.Len() != len(want.Objects) {
				t.Errorf("%s round-tripped %d objects; want %d",
					want.ConfigAddr, elem.Value.ObjectResults.Len(), len(want.Objects))
			}
			for _, obj := range want.Objects {
				var seen bool
				for _, oe := range elem.Value.ObjectResults.Elems {
					if oe.Key.String() == obj.ObjectAddr {
						seen = true
						if len(oe.Value.FailureMessages) != len(obj.FailureMessages) {
							t.Errorf("%s round-tripped %d failure message(s); want %d",
								obj.ObjectAddr, len(oe.Value.FailureMessages), len(obj.FailureMessages))
						}
					}
				}
				if !seen {
					t.Errorf("%s did not survive the round trip", obj.ObjectAddr)
				}
			}
		}
		if !found {
			t.Errorf("%s did not survive the round trip", want.ConfigAddr)
		}
	}
}

// TestNewCheckResultsRefuses pins the refusals, because each one is a
// corruption caught here instead of a statefile that fails to decode a run
// later.
func TestNewCheckResultsRefuses(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   CheckResultInput
		want string
	}{
		{"an unknown kind", CheckResultInput{ObjectKind: "widget", ConfigAddr: "var.x", Status: "pass"}, "checkable object kind"},
		{"an unknown status", CheckResultInput{ObjectKind: "var", ConfigAddr: "var.x", Status: "maybe"}, "check status"},
		{"a kind with no builder yet", CheckResultInput{ObjectKind: "check", ConfigAddr: "check.c", Status: "pass"}, "cannot be built"},
		{"an address that is not a variable", CheckResultInput{ObjectKind: "var", ConfigAddr: "random_pet.p", Status: "pass"}, "input-variable address"},
		{"an unparseable module prefix", CheckResultInput{ObjectKind: "var", ConfigAddr: "module.[.var.x", Status: "pass"}, "config address"},
		{"a bad object address", CheckResultInput{
			ObjectKind: "var", ConfigAddr: "var.x", Status: "pass",
			Objects: []CheckResultObjectInput{{ObjectAddr: "not.a.variable", Status: "pass"}},
		}, "object address"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewCheckResults([]CheckResultInput{tc.in})
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the refusal does not say %q: %v", tc.want, err)
			}
		})
	}
	if got, err := NewCheckResults(nil); got != nil || err != nil {
		t.Errorf("an empty input built %v (%v); it must be the nil section the encoder writes as null", got, err)
	}
}
