// Copyright 2023 Stefan Prodan
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

// Action holds the list of annotations for controlling
// Timoni's apply behaviour of Kubernetes resources.
Action: {
	// Force annotation for recreating immutable resources such as Kubernetes Jobs.
	Force: {
		"action.timoni.sh/force": ActionStatus.Enabled
	}
	// Recreate annotation for deleting and recreating resources on every apply,
	// even when the resource spec is unchanged (e.g. migration Jobs).
	Recreate: {
		"action.timoni.sh/recreate": ActionStatus.Enabled
	}
	// DeleteAfterHooks annotation for deleting resources as a batch after a
	// consecutive group of such resources become ready and before the next
	// normal apply set runs (e.g. migration Job + DSN ConfigMap).
	DeleteAfterHooks: {
		"action.timoni.sh/delete-after-hooks": ActionStatus.Enabled
	}
	// One-off annotation for appling resources only if they don't exist on the cluster.
	Oneoff: {
		"action.timoni.sh/one-off": ActionStatus.Enabled
	}
	// Keep annotation for preventing Timoni's garbage collector from deleting resources.
	Keep: {
		"action.timoni.sh/prune": ActionStatus.Disabled
	}
	// DisableWaiting annotation is for excluding resouces from Timoni's readiness check.
	DisableWaiting: {
		"action.timoni.sh/wait": ActionStatus.Disabled
	}
}

ActionStatus: {
	Enabled:  "enabled"
	Disabled: "disabled"
}
