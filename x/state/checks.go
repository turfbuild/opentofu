// Copyright (c) The Turf Authors
// SPDX-License-Identifier: MPL-2.0

package state

import (
	"fmt"
	"strings"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/checks"
	"github.com/opentofu/opentofu/internal/states"
)

// The statefile's check_results section, reachable from outside.
//
// The section is built by OpenTofu Core from a checks.State the graph walk
// accumulates, and neither that type nor the addrs.Map / addrs.Checkable
// machinery it rests on is nameable outside internal/. So the shim here is
// not a re-export of the types — it is a constructor in the STATEFILE's own
// vocabulary: the four strings version4.go writes ("object_kind",
// "config_addr", "status", "object_addr") in, a *states.CheckResults out. A
// caller with its own evaluator can then record what it checked without
// modelling OpenTofu's check bookkeeping, which has no meaning outside a core
// walk.
//
// Deliberately not the whole of checks.State: individual checks have no
// durable identity between runs (the type's own doc says so — reordering an
// object's rules changes nothing), so the aggregate per checkable object is
// exactly the durable part, and the durable part is what a statefile holds.

// CheckResults is the snapshot a statefile carries.
type CheckResults = states.CheckResults

// CheckResultInput is one configuration object's outcomes, flattened into the
// statefile's spelling.
type CheckResultInput struct {
	// ObjectKind is "resource", "output", "check" or "var".
	ObjectKind string
	// ConfigAddr is the configuration address: module path without instance
	// keys, then the object's own address.
	ConfigAddr string
	// Status is "pass", "fail", "error" or "unknown".
	Status  string
	Objects []CheckResultObjectInput
}

// CheckResultObjectInput is one checkable object's outcome within its
// configuration object.
type CheckResultObjectInput struct {
	// ObjectAddr is the instance address: module path WITH instance keys.
	ObjectAddr string
	Status     string
	// FailureMessages is set only for a "fail" status.
	FailureMessages []string
}

// NewCheckResults builds a statefile-ready snapshot from flattened inputs.
// Returns nil for an empty input, which is what version4.go's encoder
// normalizes an empty set to anyway — so a caller that checked nothing and a
// caller that reports nothing produce the same statefile.
//
// Every address is parsed rather than trusted: an unparseable one would be
// written into the statefile verbatim and fail to decode on the way back in,
// which is a corruption discovered a run later instead of here.
func NewCheckResults(in []CheckResultInput) (*CheckResults, error) {
	if len(in) == 0 {
		return nil, nil
	}
	ret := &states.CheckResults{
		ConfigResults: addrs.MakeMap[addrs.ConfigCheckable, *states.CheckResultAggregate](),
	}
	for _, cr := range in {
		kind, err := parseCheckableKind(cr.ObjectKind)
		if err != nil {
			return nil, err
		}
		configAddr, err := parseConfigCheckable(kind, cr.ConfigAddr)
		if err != nil {
			return nil, fmt.Errorf("check result config address %q: %w", cr.ConfigAddr, err)
		}
		status, err := parseCheckStatus(cr.Status)
		if err != nil {
			return nil, fmt.Errorf("check result for %s: %w", cr.ConfigAddr, err)
		}
		aggr := &states.CheckResultAggregate{
			Status:        status,
			ObjectResults: addrs.MakeMap[addrs.Checkable, *states.CheckResultObject](),
		}
		for _, obj := range cr.Objects {
			objAddr, err := parseCheckable(kind, obj.ObjectAddr)
			if err != nil {
				return nil, fmt.Errorf("check result object address %q: %w", obj.ObjectAddr, err)
			}
			objStatus, err := parseCheckStatus(obj.Status)
			if err != nil {
				return nil, fmt.Errorf("check result for %s: %w", obj.ObjectAddr, err)
			}
			aggr.ObjectResults.Put(objAddr, &states.CheckResultObject{
				Status:          objStatus,
				FailureMessages: obj.FailureMessages,
			})
		}
		ret.ConfigResults.Put(configAddr, aggr)
	}
	return ret, nil
}

func parseCheckableKind(kind string) (addrs.CheckableKind, error) {
	switch kind {
	case "resource":
		return addrs.CheckableResource, nil
	case "output":
		return addrs.CheckableOutputValue, nil
	case "check":
		return addrs.CheckableCheck, nil
	case "var":
		return addrs.CheckableInputVariable, nil
	default:
		return addrs.CheckableKindInvalid, fmt.Errorf("unknown checkable object kind %q", kind)
	}
}

func parseCheckStatus(status string) (checks.Status, error) {
	switch status {
	case "pass":
		return checks.StatusPass, nil
	case "fail":
		return checks.StatusFail, nil
	case "error":
		return checks.StatusError, nil
	case "unknown":
		return checks.StatusUnknown, nil
	default:
		return checks.StatusUnknown, fmt.Errorf("unknown check status %q", status)
	}
}

// parseConfigCheckable and parseCheckable are the inverses of the encoders in
// states/statefile/version4.go — only the input-variable arm is implemented,
// because it is the only kind whose address has no parser reachable from here
// and the only kind a non-core caller can currently produce. The others report
// what they are rather than guessing.
func parseConfigCheckable(kind addrs.CheckableKind, addr string) (addrs.ConfigCheckable, error) {
	if kind != addrs.CheckableInputVariable {
		return nil, fmt.Errorf("checkable kind %q cannot be built through this API yet", kind)
	}
	prefix, name, err := splitVariableAddr(addr)
	if err != nil {
		return nil, err
	}
	modInst, err := parseModulePrefix(prefix)
	if err != nil {
		return nil, err
	}
	// Module(), not the instance: a configuration object covers every instance
	// of its module, so the keys are dropped here and only here.
	return addrs.ConfigInputVariable{Module: modInst.Module(), Variable: addrs.InputVariable{Name: name}}, nil
}

func parseCheckable(kind addrs.CheckableKind, addr string) (addrs.Checkable, error) {
	if kind != addrs.CheckableInputVariable {
		return nil, fmt.Errorf("checkable kind %q cannot be built through this API yet", kind)
	}
	prefix, name, err := splitVariableAddr(addr)
	if err != nil {
		return nil, err
	}
	modInst, err := parseModulePrefix(prefix)
	if err != nil {
		return nil, err
	}
	return addrs.AbsInputVariableInstance{Module: modInst, Variable: addrs.InputVariable{Name: name}}, nil
}

// splitVariableAddr splits "module.a[0].module.b.var.x" into the module prefix
// "module.a[0].module.b" and the name "x".
func splitVariableAddr(addr string) (prefix, name string, err error) {
	idx := strings.LastIndex(addr, "var.")
	if idx < 0 {
		return "", "", fmt.Errorf("not an input-variable address")
	}
	name = addr[idx+len("var."):]
	if name == "" || strings.ContainsAny(name, ".[") {
		return "", "", fmt.Errorf("not an input-variable address")
	}
	return strings.TrimSuffix(addr[:idx], "."), name, nil
}

// parseModulePrefix parses the module-instance half through OpenTofu's own
// grammar rather than a second one written here, so an address this package
// accepts is exactly an address the statefile decoder will accept back.
func parseModulePrefix(prefix string) (addrs.ModuleInstance, error) {
	if prefix == "" {
		return addrs.RootModuleInstance, nil
	}
	parsed, diags := addrs.ParseModuleInstanceStr(prefix)
	if diags.HasErrors() {
		return nil, diags.Err()
	}
	return parsed, nil
}
