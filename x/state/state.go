// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

// Package state re-exports OpenTofu's internal state types through a stable API boundary.
// This prevents direct imports of OpenTofu internals from consuming modules.
package state

import (
	"strings"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	"github.com/opentofu/svchost"
)

// Core state types.
type State = states.State
type Module = states.Module
type Resource = states.Resource
type ResourceInstance = states.ResourceInstance
type ResourceInstanceObjectSrc = states.ResourceInstanceObjectSrc
type ResourceInstanceObject = states.ResourceInstanceObject
type OutputValue = states.OutputValue

// Object status.
type ObjectStatus = states.ObjectStatus

const (
	ObjectReady   = states.ObjectReady
	ObjectTainted = states.ObjectTainted
	ObjectPlanned = states.ObjectPlanned
)

// Deposed keys for create-before-destroy.
type DeposedKey = states.DeposedKey

const NotDeposed = states.NotDeposed

var NewDeposedKey = states.NewDeposedKey

// State constructors.
var NewState = states.NewState

// Provider configuration address (kept here because provider configs are
// consumed predominantly through the state path). Resource addressing types
// (AbsResourceInstance, ModuleInstance, IntKey, StringKey, NoKey, etc.) live
// in the sibling x/addrs package.
type AbsProviderConfig = addrs.AbsProviderConfig

// Provider address type (re-exported from registry-address).
type Provider = addrs.Provider

// State manager interfaces.
type Full = statemgr.Full
type Locker = statemgr.Locker
type LockInfo = statemgr.LockInfo
type LockError = statemgr.LockError

// State manager helpers.
var NewLockInfo = statemgr.NewLockInfo

// NewFullFake returns an in-memory statemgr.Full backed by a transient store.
// Intended for tests that need a Manager without standing up a real backend.
var NewFullFake = statemgr.NewFullFake

// NewProvider constructs a Provider from hostname, namespace, and type name.
// The hostname should be a normalized ASCII hostname (e.g. "registry.opentofu.org").
func NewProvider(hostname, namespace, typeName string) Provider {
	return addrs.NewProvider(svchost.Hostname(strings.ToLower(hostname)), namespace, typeName)
}

// RootProviderConfig creates an AbsProviderConfig for the root module with no alias.
func RootProviderConfig(provider Provider) AbsProviderConfig {
	return addrs.AbsProviderConfig{
		Module:   addrs.RootModule,
		Provider: provider,
	}
}

// ProviderConfig creates an AbsProviderConfig for the root module with an optional alias.
func ProviderConfig(provider Provider, alias string) AbsProviderConfig {
	return addrs.AbsProviderConfig{
		Module:   addrs.RootModule,
		Provider: provider,
		Alias:    alias,
	}
}
