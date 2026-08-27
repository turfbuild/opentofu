// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"testing"
)

func TestParseAbsResourceInstance_RoundTrip(t *testing.T) {
	cases := []string{
		"aws_instance.x",
		"data.aws_ami.latest",
		"aws_instance.x[0]",
		`aws_instance.x["k"]`,
		"data.aws_ami.latest[0]",
		`data.aws_ami.latest["k"]`,
		"module.foo.aws_instance.x",
		"module.foo[0].aws_instance.x",
		`module.foo["a"].aws_instance.x`,
		`module.foo[0].data.aws_s3_bucket.bar["k"]`,
		`module.foo["a"].module.bar.aws_instance.x[2]`,
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			got, err := ParseAbsResourceInstance(in)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if out := got.String(); out != in {
				t.Errorf("round-trip mismatch: in=%q out=%q", in, out)
			}
		})
	}
}

func TestParseAbsResourceInstance_DataSourceModeAndKey(t *testing.T) {
	// Regression: the pre-feb4f1a state parser hardcoded NoKey for data sources,
	// silently dropping the instance key. Confirm the canonical parser keeps it.
	got, err := ParseAbsResourceInstance(`data.aws_ami.latest["k"]`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Resource.Resource.Mode != DataResourceMode {
		t.Errorf("mode = %v, want DataResourceMode", got.Resource.Resource.Mode)
	}
	if got.Resource.Key != StringKey("k") {
		t.Errorf("key = %#v, want StringKey(\"k\")", got.Resource.Key)
	}
}

func TestParseAbsResourceInstance_Error(t *testing.T) {
	for _, in := range []string{
		"",
		"not an address",
		"module.",
		"aws_instance",
		"aws_instance.x[unclosed",
	} {
		t.Run(in, func(t *testing.T) {
			_, err := ParseAbsResourceInstance(in)
			if err == nil {
				t.Errorf("expected error for %q", in)
			}
		})
	}
}

func TestParseAbsResource(t *testing.T) {
	got, err := ParseAbsResource("module.foo.aws_instance.x")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.String() != "module.foo.aws_instance.x" {
		t.Errorf("round-trip: %q", got.String())
	}
	// Rejects instance keys.
	if _, err := ParseAbsResource("aws_instance.x[0]"); err == nil {
		t.Errorf("expected error when address carries an instance key")
	}
}

func TestParseModuleInstance(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"module.foo", "module.foo"},
		{"module.foo[0]", "module.foo[0]"},
		{`module.foo["k"]`, `module.foo["k"]`},
		{`module.foo[0].module.bar["k"]`, `module.foo[0].module.bar["k"]`},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, err := ParseModuleInstance(c.in)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if out := got.String(); out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
	if _, err := ParseModuleInstance("aws_instance.x"); err == nil {
		t.Errorf("expected error parsing a resource address as a module address")
	}
}

func TestFormatInstanceKeySuffix(t *testing.T) {
	cases := []struct {
		key  InstanceKey
		want string
	}{
		{NoKey, ""},
		{IntKey(0), "[0]"},
		{IntKey(42), "[42]"},
		{StringKey("k"), `["k"]`},
		{StringKey(`with "quote"`), `["with \"quote\""]`},
	}
	for _, c := range cases {
		if got := FormatInstanceKeySuffix(c.key); got != c.want {
			t.Errorf("FormatInstanceKeySuffix(%v) = %q, want %q", c.key, got, c.want)
		}
	}
}

func TestInstanceKeyFromAny(t *testing.T) {
	cases := []struct {
		in      any
		want    InstanceKey
		wantErr bool
	}{
		{nil, NoKey, false},
		{int(7), IntKey(7), false},
		{int64(7), IntKey(7), false},
		{float64(7), IntKey(7), false},
		{"k", StringKey("k"), false},
		{IntKey(3), IntKey(3), false},
		{StringKey("s"), StringKey("s"), false},
		{true, NoKey, true},
		{struct{}{}, NoKey, true},
	}
	for _, c := range cases {
		got, err := InstanceKeyFromAny(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("InstanceKeyFromAny(%v): err=%v wantErr=%v", c.in, err, c.wantErr)
			continue
		}
		if !c.wantErr && got != c.want {
			t.Errorf("InstanceKeyFromAny(%v) = %#v, want %#v", c.in, got, c.want)
		}
	}
}

func TestIsDataAddr(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"data.aws_ami.latest", true},
		{`data.aws_ami.latest["k"]`, true},
		{`module.foo[0].data.aws_ami.latest`, true},
		{"aws_instance.x", false},
		{"module.foo.aws_instance.x", false},
		{"not parseable", false},
	}
	for _, c := range cases {
		if got := IsDataAddr(c.in); got != c.want {
			t.Errorf("IsDataAddr(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestResourceModeClassification pins the three-way split the address predicates
// rest on. The case that matters is the last pair: an ephemeral address and a
// managed address that differ only in the block type must classify differently,
// because a caller that treats "not data" as "managed" would route an ephemeral
// declaration into the managed plan/apply lifecycle it has none of.
func TestResourceModeClassification(t *testing.T) {
	cases := []struct {
		in        string
		wantMode  ResourceMode
		wantOK    bool
		wantBlock string
	}{
		{"aws_instance.x", ManagedResourceMode, true, "resource"},
		{"data.aws_ami.latest", DataResourceMode, true, "data"},
		{"ephemeral.vault_kv_secret_v2.creds", EphemeralResourceMode, true, "ephemeral"},
		{`ephemeral.vault_kv_secret_v2.creds["k"]`, EphemeralResourceMode, true, "ephemeral"},
		{"module.foo[0].ephemeral.vault_kv_secret_v2.creds", EphemeralResourceMode, true, "ephemeral"},
		{"not parseable", InvalidResourceMode, false, "unknown"},
	}
	for _, c := range cases {
		mode, ok := ResourceModeOf(c.in)
		if ok != c.wantOK || mode != c.wantMode {
			t.Errorf("ResourceModeOf(%q) = %v, %v; want %v, %v", c.in, mode, ok, c.wantMode, c.wantOK)
		}
		if got := ResourceModeBlockName(mode); got != c.wantBlock {
			t.Errorf("ResourceModeBlockName for %q = %q, want %q", c.in, got, c.wantBlock)
		}
	}

	// The predicates must not overlap: exactly one of them is true for any
	// address that parses at all.
	for _, addr := range []string{"aws_instance.x", "data.aws_ami.latest", "ephemeral.vault_kv_secret_v2.creds"} {
		if IsDataAddr(addr) && IsEphemeralAddr(addr) {
			t.Errorf("%q classified as both data and ephemeral", addr)
		}
	}
	if !IsEphemeralAddr("ephemeral.vault_kv_secret_v2.creds") {
		t.Error("IsEphemeralAddr rejected an ephemeral address")
	}
	if IsEphemeralAddr("vault_kv_secret_v2.creds") {
		t.Error("IsEphemeralAddr accepted a managed address of the same type and name")
	}
}
