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
	parsed, err := ParseProviderSourceString("aws")
	if err != nil {
		t.Fatal(err)
	}
	if got != parsed {
		t.Errorf("NewProvider(default host, hashicorp, aws) = %q, want it equal to the parsed %q", got, parsed)
	}
}

func TestProviderConfig(t *testing.T) {
	p := NewProvider(DefaultProviderRegistryHost.String(), "hashicorp", "aws")
	if got := RootProviderConfig(p).String(); got != `provider["registry.opentofu.org/hashicorp/aws"]` {
		t.Errorf("RootProviderConfig = %s", got)
	}
	if got := ProviderConfig(p, "west").String(); got != `provider["registry.opentofu.org/hashicorp/aws"].west` {
		t.Errorf("ProviderConfig(west) = %s", got)
	}
}
