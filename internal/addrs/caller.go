// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

// Caller is the address of the special object "caller" that is available
// inside an action's configuration when the action is evaluated for a
// resource's action_trigger, where it behaves as an alias for the
// triggering resource instance.
const Caller callerT = 0

type callerT int

func (c callerT) referenceableSigil() {
}

func (c callerT) String() string {
	return "caller"
}

func (c callerT) UniqueKey() UniqueKey {
	return Caller // Caller is its own UniqueKey
}

func (c callerT) uniqueKeySigil() {}
