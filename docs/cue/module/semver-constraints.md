# Kubernetes Version Constraints

At apply-time, Timoni reads the Kubernetes version from the live cluster and sets the version
string as the value of the `kubeVersion` field in the module's `#Config` definition.

With the `timoniv1.#SemVer` helper, you can parse the Kubernetes version string to extract the
major and minor version numbers.

```cue
#Config: {
	kubeVersion!: string
	clusterVersion: timoniv1.#SemVer & {
		#Version: kubeVersion
		#Minimum: "0.0.0"
	}
}

```

## Enforcing a minimum supported version

To enforce a minimum supported version for your module, set a constraint for the minor
version e.g. `#Minimum: "1.20.0"`.

To test the constraint, you can use the `TIMONI_KUBE_VERSION` env var
with `timoni mod vet` and `timoni build`.

```console
$ TIMONI_KUBE_VERSION=1.19.0 timoni mod vet
validation failed: clusterVersion.minor: invalid value 19 (out of bound >=20)
```

## Conditionally enabling features based on cluster version

You can use the `clusterVersion.minor` value to conditionally apply certain Kubernetes resources
based on the cluster version.

For example, the `FlowSchema` v1 API was introduced in Kubernetes 1.29. To apply this resource
only on clusters running Kubernetes 1.19 or newer,
you can use the following condition in your module's `#Instance` definition:

```cue
#Instance: {
	config: #Config

	if config.clusterVersion.minor >= 29 {
		objects: flowSchema: #FlowSchema & {_config: config}
	}
}

```