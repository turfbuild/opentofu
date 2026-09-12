// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsonplan

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/plans"
)

// TestMarshalActionInvocationsProviderName pins the shape of an invocation's
// provider_name against the shape MarshalResourceChanges gives a resource's:
// the fully-qualified address, hostname included.
//
// The "registry host other than the default" case is the load-bearing one. A
// fixture on the default registry cannot distinguish a renderer that qualifies
// the address from one that drops the host and happens to agree, because the
// two spellings of a default-registry provider differ only by the prefix the
// dropping renderer omits. Only a non-default host makes the difference visible.
func TestMarshalActionInvocationsProviderName(t *testing.T) {
	tests := map[string]struct {
		Hostname  string
		Namespace string
		Name      string
		Want      string
	}{
		"registry host other than the default": {
			Hostname:  "example.com",
			Namespace: "example",
			Name:      "mock",
			Want:      "example.com/example/mock",
		},
		"the default registry, named explicitly": {
			Hostname:  "registry.opentofu.org",
			Namespace: "hashicorp",
			Name:      "mock",
			Want:      "registry.opentofu.org/hashicorp/mock",
		},
		// An empty hostname means the default registry (the field's contract),
		// so it has to render identically to the explicit spelling above --
		// otherwise one provider reaches a consumer under two names.
		"an empty hostname resolves to the default registry": {
			Hostname:  "",
			Namespace: "hashicorp",
			Name:      "mock",
			Want:      "registry.opentofu.org/hashicorp/mock",
		},
		// A host that no registry serves is still part of the address: an
		// invocation whose provider runs in-process wears one, and qualifying
		// it against the default registry would name a provider that does not
		// exist.
		"a host no registry serves is carried through": {
			Hostname:  "pseudo.example.net",
			Namespace: "example",
			Name:      "builtin",
			Want:      "pseudo.example.net/example/builtin",
		},
		"a unicode hostname renders in its display form": {
			Hostname:  "xn--eckwd4c7cu47r2wf.example.com",
			Namespace: "example",
			Name:      "mock",
			Want:      "ドメイン名例.example.com/example/mock",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := MarshalActionInvocations([]*plans.ActionInvocationInstanceSrc{{
				Addr:              "action.example_act.one",
				Type:              "example_act",
				ProviderHostname:  test.Hostname,
				ProviderNamespace: test.Namespace,
				ProviderName:      test.Name,
			}}, nil)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if len(got) != 1 {
				t.Fatalf("wrong number of invocations %d; want 1", len(got))
			}
			if got[0].ProviderName != test.Want {
				t.Errorf("wrong provider_name\ngot:  %s\nwant: %s", got[0].ProviderName, test.Want)
			}
		})
	}
}

// TestMarshalActionInvocationsProviderNameAgreesWithResourceChanges is the
// cross-array fact the per-case table above only implies: one plan document
// must not name a single provider two ways. It asserts against the renderer
// resource_changes actually uses rather than against a literal, so the two stay
// tied together if either moves.
func TestMarshalActionInvocationsProviderNameAgreesWithResourceChanges(t *testing.T) {
	provider := addrs.Provider{Hostname: "example.com", Namespace: "example", Type: "mock"}

	got, err := MarshalActionInvocations([]*plans.ActionInvocationInstanceSrc{{
		Addr:              "action.mock_act.one",
		Type:              "mock_act",
		ProviderHostname:  provider.Hostname.ForDisplay(),
		ProviderNamespace: provider.Namespace,
		ProviderName:      provider.Type,
	}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if got[0].ProviderName != provider.String() {
		t.Errorf("action provider_name disagrees with the resource-change rendering\ngot:  %s\nwant: %s", got[0].ProviderName, provider.String())
	}
}

// TestMarshalActionInvocationsWithoutAProvider covers the invocation that names
// no provider at all: the field is tagged omitempty, but a renderer that
// concatenates the parts emits a bare "/" and omitempty never fires.
func TestMarshalActionInvocationsWithoutAProvider(t *testing.T) {
	got, err := MarshalActionInvocations([]*plans.ActionInvocationInstanceSrc{{
		Addr: "action.example_act.one",
		Type: "example_act",
	}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	raw, err := json.Marshal(got[0])
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if strings.Contains(string(raw), "provider_name") {
		t.Errorf("provider_name should be omitted entirely; got %s", raw)
	}
}
