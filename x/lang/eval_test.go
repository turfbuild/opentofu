// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package lang

import (
	"reflect"
	"testing"

	"github.com/zclconf/go-cty/cty"
)

func TestFindUnknowns(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want []string
	}{
		{
			name: "wholly known map",
			in: map[string]any{
				"name": "foo",
				"size": int64(3),
				"tags": map[string]any{"env": "dev"},
				"list": []any{"a", "b"},
			},
			want: nil,
		},
		{
			name: "top level unknown leaf",
			in:   UnknownValue,
			want: []string{"<root>"},
		},
		{
			name: "unknown at top of map",
			in: map[string]any{
				"id":   UnknownValue,
				"name": "foo",
			},
			want: []string{"id"},
		},
		{
			name: "unknown inside nested map",
			in: map[string]any{
				"network": map[string]any{
					"cidr": UnknownValue,
					"name": "main",
				},
			},
			want: []string{"network.cidr"},
		},
		{
			name: "unknown inside list",
			in: map[string]any{
				"items": []any{"ok", UnknownValue, "ok"},
			},
			want: []string{"items[1]"},
		},
		{
			name: "unknown inside list of maps",
			in: map[string]any{
				"subnets": []any{
					map[string]any{"cidr": "10.0.0.0/24"},
					map[string]any{"cidr": UnknownValue},
				},
			},
			want: []string{"subnets[1].cidr"},
		},
		{
			name: "multiple unknowns sorted",
			in: map[string]any{
				"z": UnknownValue,
				"a": UnknownValue,
				"m": map[string]any{"x": UnknownValue},
			},
			want: []string{"a", "m.x", "z"},
		},
		{
			name: "non-unknown string ignored",
			in: map[string]any{
				"label": "__cty_known__",
			},
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FindUnknowns(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("FindUnknowns() = %#v; want %#v", got, tc.want)
			}
		})
	}
}

// TestCtyGoRoundTrip pins the conversions a caller holding JSON-shaped values
// relies on: whole numbers come back as int64, unknowns as UnknownValue, and
// GoToCty reads UnknownValue back as unknown.
func TestCtyGoRoundTrip(t *testing.T) {
	in := cty.ObjectVal(map[string]cty.Value{
		"name":  cty.StringVal("web"),
		"count": cty.NumberIntVal(3),
		"ratio": cty.NumberFloatVal(0.5),
		"on":    cty.True,
		"tags":  cty.ListVal([]cty.Value{cty.StringVal("a")}),
		"later": cty.UnknownVal(cty.String),
		"none":  cty.NullVal(cty.String),
	})
	got := CtyToGo(in)
	want := map[string]any{
		"name": "web", "count": int64(3), "ratio": 0.5, "on": true,
		"tags": []any{"a"}, "later": UnknownValue, "none": nil,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CtyToGo = %#v, want %#v", got, want)
	}

	back := GoToCty(got)
	if back.GetAttr("later").IsKnown() {
		t.Error("UnknownValue did not come back unknown")
	}
	if !back.GetAttr("count").RawEquals(cty.NumberIntVal(3)) {
		t.Errorf("count came back as %#v", back.GetAttr("count"))
	}
}
