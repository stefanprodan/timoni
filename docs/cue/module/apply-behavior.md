# Control the Apply Behaviour

Timoni allows changing the default apply behaviour of Kubernetes resources
with the `action.timoni.sh` annotations.

## Annotations

| CUE                      | Generated YAML                      |
|--------------------------|-------------------------------------|
| `timoniv1.Action.Force`  | `action.timoni.sh/force: enabled`   |
| `timoniv1.Action.OneOff` | `action.timoni.sh/one-off: enabled` |
| `timoniv1.Action.Keep`   | `action.timoni.sh/prune: disabled`  |

### Force Apply

To recreate immutable resources such as Kubernetes Jobs,
these resources can be annotated with `action.timoni.sh/force: "enabled"`.

Example:

```cue
package templates

import (
	batchv1 "k8s.io/api/batch/v1"
	timoniv1 "timoni.sh/core/v1alpha1"
)

#TestJob: batchv1.#Job & {
	_config:    #Config
	apiVersion: "batch/v1"
	kind:       "Job"
	metadata: timoniv1.#MetaComponent & {
		#Meta:      _config.metadata
		#Component: "test"
	}
	metadata: annotations: timoniv1.Action.Force
	spec: {...}
}

```

### One-Off Apply

To apply resources only if they don't exist on the cluster,
these resources can be annotated with `action.timoni.sh/one-off: "enabled"`.

Example:

```cue
package templates

import (
	batchv1 "k8s.io/api/batch/v1"
	timoniv1 "timoni.sh/core/v1alpha1"
)

#InstallJob: batchv1.#Job & {
	_config:    #Config
	apiVersion: "batch/v1"
	kind:       "Job"
	metadata: timoniv1.#MetaComponent & {
		#Meta:      _config.metadata
		#Component: "install"
	}
	metadata: annotations: timoniv1.Action.OneOff
	spec: {...}
}

```

### Disable Pruning

To prevent Timoni's garbage collector from deleting certain
resources such as Kubernetes Persistent Volumes,
these resources can be annotated with `action.timoni.sh/prune: "disabled"`.


Example:

```cue
package templates

import (
	corev1 "k8s.io/api/core/v1"
	timoniv1 "timoni.sh/core/v1alpha1"
)

#DatabasePVC: corev1.#PersistentVolumeClaim & {
	_config:    #Config
	apiVersion: "v1"
	kind:       "PersistentVolumeClaim"
	metadata: timoniv1.#MetaComponent & {
		#Meta:      _config.metadata
		#Component: "database"
	}
	metadata: annotations: timoniv1.Action.Keep
	spec: {...}
}

```
