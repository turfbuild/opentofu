// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"testing"
)

func TestParseProviderSourceString(t *testing.T) {
	cases := []struct {
		source string
		want   string
	}{
		{"aws", "registry.opentofu.org/hashicorp/aws"},
		{"hashicorp/aws", "registry.opentofu.org/hashicorp/aws"},
		{"registry.opentofu.org/hashicorp/aws", "registry.opentofu.org/hashicorp/aws"},
		{"example.com/foo/bar", "example.com/foo/bar"},
		{"hashicorp/google-beta", "registry.opentofu.org/hashicorp/google-beta"},
	}
	for _, tc := range cases {
		got, err := ParseProviderSourceString(tc.source)
		if err != nil {
			t.Errorf("ParseProviderSourceString(%q): unexpected error: %s", tc.source, err)
			continue
		}
		if got.String() != tc.want {
			t.Errorf("ParseProviderSourceString(%q) = %q, want %q", tc.source, got, tc.want)
		}
	}
}

func TestParseProviderSourceString_Error(t *testing.T) {
	for _, source := range []string{"", "a/b/c/d", "hashicorp/", "hashicorp/AWS!"} {
		if got, err := ParseProviderSourceString(source); err == nil {
			t.Errorf("ParseProviderSourceString(%q) = %q, want an error", source, got)
		}
	}
}

func TestImpliedProviderForUnqualifiedType(t *testing.T) {
	// The one name that implies a builtin provider rather than a registry one.
	if got := ImpliedProviderForUnqualifiedType("terraform"); got.String() != "terraform.io/builtin/terraform" {
		t.Errorf("terraform implied %q, want the builtin provider", got)
	}
	if got := ImpliedProviderForUnqualifiedType("random"); got.String() != "registry.opentofu.org/hashicorp/random" {
		t.Errorf("random implied %q, want the default-registry provider", got)
	}
}

func TestImpliedProviderCutsAtFirstUnderscore(t *testing.T) {
	cases := map[string]string{
		"random_pet":           "random",
		"aws_s3_bucket":        "aws",
		"tfcoremock":           "tfcoremock",
		"google-beta_instance": "google-beta",
		"_leading":             "",
	}
	for typeName, want := range cases {
		if got := (Resource{Type: typeName}).ImpliedProvider(); got != want {
			t.Errorf("Resource{Type: %q}.ImpliedProvider() = %q, want %q", typeName, got, want)
		}
	}
}

func TestNewProviderNormalizesHostname(t *testing.T) {
	got := NewProvider("Registry.OpenTofu.Org", "hashicorp", "aws")
	if got.String() != "registry.opentofu.org/hashicorp/aws" {
		t.Errorf("NewProvider with a mixed-case host = %q, want the normalized form", got)
	}
	if got != NewDefaultProvider("aws") {
		t.Errorf("NewProvider(default host, hashicorp, aws) = %q, want it equal to NewDefaultProvider(\"aws\")", got)
	}
}
