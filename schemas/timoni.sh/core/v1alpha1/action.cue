// Copyright 2023 Stefan Prodan
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

// Action holds a list of annotations for controlling
// Timoni's apply behaviour of Kubernetes resources.
action: {
	// Force annotation for recreating immutable resources such as Kubernetes Jobs.
	force: {
		"action.timoni.sh/force": enabled
	}
	// One-off annotation for appling resources only if they don't exist on the cluster.
	oneoff: {
		"action.timoni.sh/one-off": enabled
	}
	// Keep annotation for preventing Timoni's garbage collector from deleting resources.
	keep: {
		"action.timoni.sh/prune": disabled
	}
}

enabled:  "enabled"
disabled: "disabled"
