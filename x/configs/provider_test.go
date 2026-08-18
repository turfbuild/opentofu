// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"testing"
)

func TestParseProviderConfigCompactStr(t *testing.T) {
	cases := []struct {
		ref       string
		wantLocal string
		wantAlias string
	}{
		{"aws", "aws", ""},
		{"aws.west", "aws", "west"},
		{"google-beta", "google-beta", ""},
		{"google-beta.eu", "google-beta", "eu"},
	}
	for _, tc := range cases {
		got, err := ParseProviderConfigCompactStr(tc.ref)
		if err != nil {
			t.Errorf("ParseProviderConfigCompactStr(%q): unexpected error: %s", tc.ref, err)
			continue
		}
		if got.LocalName != tc.wantLocal || got.Alias != tc.wantAlias {
			t.Errorf("ParseProviderConfigCompactStr(%q) = {%q, %q}, want {%q, %q}",
				tc.ref, got.LocalName, got.Alias, tc.wantLocal, tc.wantAlias)
		}
	}
}

func TestParseProviderConfigCompactStr_Error(t *testing.T) {
	for _, ref := range []string{"", "aws.west.extra", "aws[0]", ".west", "aws."} {
		if got, err := ParseProviderConfigCompactStr(ref); err == nil {
			t.Errorf("ParseProviderConfigCompactStr(%q) = %#v, want an error", ref, got)
		}
	}
}
