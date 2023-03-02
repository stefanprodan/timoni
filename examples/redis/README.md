# Redis

[Redis](https://redis.io/) is an open source, in-memory data store used by
millions of developers as a database, cache, streaming engine, and message broker.

This module is for deploying a Redis master-replica cluster as a LRU cache
with persistent storage and
[append-only](https://redis.io/docs/management/persistence/#append-only-file)
durable strategy.

## Module Repository

This module is available on GitHub Container Registry at
[ghcr.io/stefanprodan/modules/redis](https://github.com/stefanprodan/timoni/pkgs/container/modules%2Fredis).

## Install

To create an instance using the default values:

```shell
timoni -n default apply redis oci://ghcr.io/stefanprodan/modules/redis
```

The Redis cluster can be accessed using the following Kubernetes Services:

- `tcp://redis:6379` read-write endpoint for the Redis master
- `tcp://redis-readonly:6379` read-only endpoint with load balancing across Redis replicas

To install a specific module version:

```shell
timoni -n default apply redis oci://ghcr.io/stefanprodan/modules/redis -v 7.0.9
```

To change the [default configuration](#configuration),
create one or more `values.cue` files and apply them to the instance.

For example, create a file `my-values.cue` with the following content:

```cue
values: {
	maxmemory: 1024
	readonly: replicas: 2
	persistence: {
		enabled:      true
		storageClass: "standard"
		size:         "8Gi"
	}
}
```

And apply the values with:

```shell
timoni -n default apply redis oci://ghcr.io/stefanprodan/modules/redis \
--values ./my-values.cue
```

## Upgrade

To upgrade, run the [apply](#install) command for a different version and/or different values.

```shell
timoni -n default apply redis oci://ghcr.io/stefanprodan/modules/redis \
--version latest \
--values ./my-values.cue \
--values ./my-other-values.cue
```

On config changes, Timoni will first upgrade the Redis master,
will wait for it to become ready, then it will upgrade the read-only replicas.

## Uninstall

To uninstall an instance and delete all its Kubernetes resources:

```shell
timoni -n default delete redis
```

## Configuration

### Redis values

| Key                          | Type     | Default    | Description                                                                                                     |
|------------------------------|----------|------------|-----------------------------------------------------------------------------------------------------------------|
| `maxmemory:`                 | `int`    | `512`      | Redis max memory in Mi (this is also used to set `resources.limits.memory`)                                     |
| `readonly: replicas:`        | `int`    | `1`        | Number of Redis read-only replicas                                                                              |
| `persistence: enabled:`      | `bool`   | `true`     | Enable persistent storage for the Redis master node                                                             |
| `persistence: storageClass:` | `string` | `standard` | The [PersistentVolumeClaim](https://kubernetes.io/docs/concepts/storage/persistent-volumes/) storage class name |
| `persistence: size:`         | `string` | `8Gi`      | The persistent volume size                                                                                      |

### General values

| Key                          | Type                                    | Default                | Description                                                                                                                                  |
|------------------------------|-----------------------------------------|------------------------|----------------------------------------------------------------------------------------------------------------------------------------------|
| `image: tag:`                | `string`                                | `<latest version>`     | Container image tag                                                                                                                          |
| `image: repository:`         | `string`                                | `docker.io/redis`      | Container image repository                                                                                                                   |
| `image: pullPolicy:`         | `string`                                | `IfNotPresent`         | [Kubernetes image pull policy](https://kubernetes.io/docs/concepts/containers/images/#image-pull-policy)                                     |
| `metadata: labels:`          | `{[ string]: string}`                   | `{}`                   | Common labels for all resources                                                                                                              |
| `metadata: annotations:`     | `{[ string]: string}`                   | `{}`                   | Common annotations for all resources                                                                                                         |
| `podAnnotations:`            | `{[ string]: string}`                   | `{}`                   | Annotations applied to pods                                                                                                                  |
| `imagePullSecrets:`          | `[...corev1.LocalObjectReference]`      | `[]`                   | [Kubernetes image pull secrets](https://kubernetes.io/docs/concepts/containers/images/#specifying-imagepullsecrets-on-a-pod)                 |
| `tolerations:`               | `[ ...corev1.#Toleration]`              | `[]`                   | [Kubernetes toleration](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration)                                        |
| `affinity:`                  | `corev1.#Affinity`                      | `{}`                   | [Kubernetes affinity and anti-affinity](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#affinity-and-anti-affinity) |
| `resources:`                 | `corev1.#ResourceRequirements`          | `{}`                   | [Kubernetes resource requests and limits](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers)                     |
| `topologySpreadConstraints:` | `[...corev1.#TopologySpreadConstraint]` | `[]`                   | [Kubernetes pod topology spread constraints](https://kubernetes.io/docs/concepts/scheduling-eviction/topology-spread-constraints)            |
| `podSecurityContext:`        | `corev1.#PodSecurityContext`            | `{runAsUser: 1001}`    | [Kubernetes pod security context](https://kubernetes.io/docs/tasks/configure-pod-container/security-context)                                 |
| `securityContext:`           | `corev1.#SecurityContext`               | `{runAsNonRoot: true}` | [Kubernetes container security context](https://kubernetes.io/docs/tasks/configure-pod-container/security-context)                           |
