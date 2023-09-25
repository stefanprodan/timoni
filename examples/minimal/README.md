# minimal

A [timoni.sh](http://timoni.sh) module for deploying minimal to Kubernetes clusters.

## Install

To create an instance using the default values:

```shell
timoni -n default apply minimal oci://<container-registry-url>
```

To change the [default configuration](#configuration),
create one or more `values.cue` files and apply them to the instance.

For example, create a file `my-values.cue` with the following content:

```cue
values: {
	resources: requests: {
		cpu:    "100m"
		memory: "128Mi"
	}
}
```

And apply the values with:

```shell
timoni -n default apply minimal oci://<container-registry-url> \
--values ./my-values.cue
```

## Uninstall

To uninstall an instance and delete all its Kubernetes resources:

```shell
timoni -n default delete minimal
```

## Configuration

### General values

| Key                          | Type                                    | Default                    | Description                                                                                                                                  |
|------------------------------|-----------------------------------------|----------------------------|----------------------------------------------------------------------------------------------------------------------------------------------|
| `image: tag:`                | `string`                                | `<latest version>`         | Container image tag                                                                                                                          |
| `image: digest:`             | `string`                                | `<latest digest>`          | Container image digest, takes precedence over `tag` when specified                                                                           |
| `image: repository:`         | `string`                                | `cgr.dev/chainguard/nginx` | Container image repository                                                                                                                   |
| `image: pullPolicy:`         | `string`                                | `IfNotPresent`             | [Kubernetes image pull policy](https://kubernetes.io/docs/concepts/containers/images/#image-pull-policy)                                     |
| `metadata: labels:`          | `{[ string]: string}`                   | `{}`                       | Common labels for all resources                                                                                                              |
| `metadata: annotations:`     | `{[ string]: string}`                   | `{}`                       | Common annotations for all resources                                                                                                         |
| `podAnnotations:`            | `{[ string]: string}`                   | `{}`                       | Annotations applied to pods                                                                                                                  |
| `imagePullSecrets:`          | `[...corev1.LocalObjectReference]`      | `[]`                       | [Kubernetes image pull secrets](https://kubernetes.io/docs/concepts/containers/images/#specifying-imagepullsecrets-on-a-pod)                 |
| `tolerations:`               | `[ ...corev1.#Toleration]`              | `[]`                       | [Kubernetes toleration](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration)                                        |
| `affinity:`                  | `corev1.#Affinity`                      | `{}`                       | [Kubernetes affinity and anti-affinity](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#affinity-and-anti-affinity) |
| `resources:`                 | `corev1.#ResourceRequirements`          | `{}`                       | [Kubernetes resource requests and limits](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers)                     |
| `topologySpreadConstraints:` | `[...corev1.#TopologySpreadConstraint]` | `[]`                       | [Kubernetes pod topology spread constraints](https://kubernetes.io/docs/concepts/scheduling-eviction/topology-spread-constraints)            |
| `podSecurityContext:`        | `corev1.#PodSecurityContext`            | `{}`                       | [Kubernetes pod security context](https://kubernetes.io/docs/tasks/configure-pod-container/security-context)                                 |
| `securityContext:`           | `corev1.#SecurityContext`               | `{}`                       | [Kubernetes container security context](https://kubernetes.io/docs/tasks/configure-pod-container/security-context)                           |

#### Recommended values

Comply with the restricted [Kubernetes pod security standard](https://kubernetes.io/docs/concepts/security/pod-security-standards/):

```cue
values: {
	podSecurityContext: {
		runAsUser:  65532
		runAsGroup: 65532
		fsGroup:    65532
	}
	securityContext: {
		allowPrivilegeEscalation: false
		readOnlyRootFilesystem:   false
		runAsNonRoot:             true
		capabilities: drop: ["ALL"]
		seccompProfile: type: "RuntimeDefault"
	}
}
```
