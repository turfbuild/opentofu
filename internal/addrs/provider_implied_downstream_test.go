// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package addrs

import "testing"

// TestImpliedProviderForUnqualifiedTypeDownstream pins the downstream rule
// beside upstream's: "turf" implies a built-in provider the way "terraform"
// does, and every other name still implies the default registry namespace.
func TestImpliedProviderForUnqualifiedTypeDownstream(t *testing.T) {
	tests := []struct {
		typeName string
		want     Provider
	}{
		{"terraform", NewBuiltInProvider("terraform")},
		{"turf", NewBuiltInProvider("turf")},
		{"turfish", NewDefaultProvider("turfish")},
		{"null", NewDefaultProvider("null")},
	}
	for _, test := range tests {
		got := ImpliedProviderForUnqualifiedType(test.typeName)
		if got != test.want {
			t.Errorf("ImpliedProviderForUnqualifiedType(%q) = %s, want %s", test.typeName, got, test.want)
		}
		if got.IsBuiltIn() != test.want.IsBuiltIn() {
			t.Errorf("ImpliedProviderForUnqualifiedType(%q).IsBuiltIn() = %t, want %t", test.typeName, got.IsBuiltIn(), test.want.IsBuiltIn())
		}
	}
}
