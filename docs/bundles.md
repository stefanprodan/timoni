# Bundles

Timoni bundles offer a declarative way of managing the lifecycle
of applications and their infra dependencies.

A Timoni bundle is a CUE file for defining a group of instances
together with their values and module references.

## Example

The following is an example of a Bundle that defines a Redis
master-replica cluster and a podinfo instance connected to the Redis instance.

```cue
bundle: {
	apiVersion: "v1alpha1"
	instances: {
		redis: {
			module: {
				url:     "oci://ghcr.io/stefanprodan/modules/redis"
				version: "7.0.10"
			}
			namespace: "podinfo"
			values: maxmemory: 256
		}
		podinfo: {
			module: url:     "oci://ghcr.io/stefanprodan/modules/podinfo"
			module: version: "6.3.5"
			namespace: "podinfo"
			values: caching: {
				enabled:  true
				redisURL: "tcp://redis:6379"
			}
		}
	}
}
```

For the above example, Timoni performs the following actions at apply time:

- Validates that the Bundle definition is in conformance with the API version specified by `apiVersion`.
- For each instance, it fetches the module version from the registry using the `module.url`
  as the artifact repository address and the `module.version` as the artifact tag.
- Creates the Kubernetes namespaces if they don't exist.
- For each instance, it builds, validates and creates the Kubernetes resources using the specified `values`.
- The list of managed resources along with the module reference and values are stored in the cluster in a
  Kubernetes Secret, in the same namespace with the instance.
- If an instance already exists, Timoni performs a server-side apply dry-run to detect changes and
  applies only the resources with divergent state.
- If previously applied resources are missing from the current revision, these
  resources are deleted from the cluster when the apply flag `--prune` is set to `true`.
- If the apply flag `--wait` is set to `true`, Timoni will wait for each instance's resources to become ready.

You can run this example by saving the Bundle into `podinfo.bundle.cue`.

Apply the Bundle on the cluster:

=== "command"

      ```sh
      timoni bundle apply -f podinfo.bundle.cue
      ```

=== "output"

      ```text
      applying instance redis
      pulling oci://ghcr.io/stefanprodan/modules/redis:7.0.10
      using module timoni.sh/redis version 7.0.10
      installing redis in namespace podinfo
      Namespace/podinfo created
      applying master
      ServiceAccount/podinfo/redis created
      ConfigMap/podinfo/redis created
      Service/podinfo/redis created
      Deployment/podinfo/redis-master created
      PersistentVolumeClaim/podinfo/redis-master created
      waiting for 5 resource(s) to become ready...
      resources are ready
      applying replica
      Service/podinfo/redis-readonly created
      Deployment/podinfo/redis-replica created
      waiting for 2 resource(s) to become ready...
      resources are ready
      applying instance podinfo
      pulling oci://ghcr.io/stefanprodan/modules/podinfo:6.3.5
      using module timoni.sh/podinfo version 6.3.5
      installing podinfo in namespace podinfo
      ServiceAccount/podinfo/podinfo created
      Service/podinfo/podinfo created
      Deployment/podinfo/podinfo created
      waiting for 3 resource(s) to become ready...
      resources are ready
      ```

List the instances in the `podinfo` namespace:

=== "command"

      ```sh
      timoni list -n podinfo
      ```

=== "output"

     ```text
     NAME    MODULE                                          VERSION LAST APPLIED         
     podinfo oci://ghcr.io/stefanprodan/modules/podinfo      6.3.5   2023-04-10T16:20:07Z    
     redis   oci://ghcr.io/stefanprodan/modules/redis        7.0.10  2023-04-10T16:20:00Z
     ```

List the instance resources and their rollout status:

=== "command"

      ```sh
      timoni status redis -n podinfo
      ```

=== "output"

     ```text
     ServiceAccount/podinfo/redis Current Resource is current
     ConfigMap/podinfo/redis Current Resource is always ready
     Service/podinfo/redis Current Service is ready
     Service/podinfo/redis-readonly Current Service is ready
     Deployment/podinfo/redis-master Current Deployment is available. Replicas: 1
     Deployment/podinfo/redis-replica Current Deployment is available. Replicas: 1
     PersistentVolumeClaim/podinfo/redis-master Current PVC is Bound
     ```

See an instance module reference and its digest:

=== "command"

      ```sh
      timoni inspect module redis -n podinfo
      ```

=== "output"

     ```text
     name: timoni.sh/redis
     repository: oci://ghcr.io/stefanprodan/modules/redis
     version: 7.0.10
     digest: sha256:7fcb6f6918902c0dedc2ba4545fbdeca119a04644a53400af15b977e3921c600
     ```

## Writing a Bundle spec

A Bundle file must contain a definition that matches the following schema:

```cue
#Bundle: {
	apiVersion: string
	instances: [string]: {
		module: {
			url:     string
			digest?: string
			version: *"latest" | string
		}
		namespace: string
		values: {...}
	}
}
```

Bundle files can contain arithmetic
[operations](https://cuelang.org/docs/tutorials/tour/expressions/operators/),
[string interpolation](https://cuelang.org/docs/tutorials/tour/expressions/interpolation/)
and everything else that CUE [std lib](https://cuelang.org/docs/references/spec/) supports.

### API version

The `apiVersion` is a required field that specifies the version of the Bundle schema.

Currently, the only supported value is `v1alpha1`.

### Instances

The `instances` array is a required field that specifies the list of Instances part of this Bundle.

A Bundle must contain at least one instance with the following required fields:

```cue
bundle: {
	apiVersion: "v1alpha1"
	instances: {
		podinfo: {
			module: url: "oci://ghcr.io/stefanprodan/modules/podinfo"
			namespace: "podinfo"
		}
	}
}
```

### Instance Module

The `instance.module` is a required field that specifies the OCI URL, version and/or digest
of the instance's module.

#### URL

The `instance.module.url` is a required field that specifies the OCI repository address
where the module is published. The `url` field must be in the format `oci://<registry-host>/<repo-name>`.

#### Version

The `instance.module.version` is an optional field that specifies the version number of the module.
The version number must follow [Timoni's semantic versioning](module-semver.md).
When not specified, the version defaults to `latest`, which pulls the module OCI artifact tagged as latest.

```cue
module: {
	url: "oci://ghcr.io/stefanprodan/modules/podinfo"
	version: "6.3.5"
}
```

!!! tip "Default version"

    When not specified, the version defaults to `latest`, which pulls the module OCI artifact tagged as latest.
    Note that using `version: "latest"` is not recommended for production system, unless you also specify a digest.  

#### Digest

The `instance.module.digest` is an optional field that specifies the OCI digest of the module.

```cue
module: {
	url:    "oci://ghcr.io/stefanprodan/modules/podinfo"
	digest: "sha256:5c3fd5ef3cdc9d742091ddc1e2546084369069ea96a076c33c1f51374a8d6325"
}
```

When both the version number and the digest are specified, Timoni will verify that the
upstream digest of the version matches the specified `instance.module.digest`.

```cue
module: {
	url:    "oci://ghcr.io/stefanprodan/modules/podinfo"
	version: "6.3.5"
	digest: "sha256:5c3fd5ef3cdc9d742091ddc1e2546084369069ea96a076c33c1f51374a8d6325"
}
```

If the version is set to `latest` and a digest is specified, Timoni will ignore the version
and will pull the module by its OCI digest.

### Instance Namespace

The `instance.module` is a required field that specifies the Kubernetes namespace where the instance is created.

If the specified namespace does not exist, Timoni will first create the namespace,
then it will apply the instance's resources in that namespace.

### Instance Values

The `instance.values` is an optional field that specifies custom values used to configure the instance.

At apply time, Timoni merges the custom values with the defaults,
validates the final values against the config schema and creates the instance.

## Working with Bundles

### Install and Upgrade

To install or upgrade the instances defined in a Bundle file,
you can use the `timoni bundle apply` command.

Example:

```shell
timoni bundle apply -f bundle.cue
```

The apply command performs the following actions for each instance:

- Pulls the module version from the specified container registry.
- If the registry is private, uses the credentials found in `~/.docker/config.json`.
- If the registry credentials are specified with `--creds`, these take priority over the docker ones.
- Creates the specified `--namespace` if it doesn't exist.
- Merges the custom values supplied in the Bundle with the default values found in the module.
- Builds the module by passing the instance name, namespace and values.
- Labels the resulting Kubernetes resources with the instance name and namespace.
- Applies the Kubernetes resources on the cluster.
- Creates or updates the instance inventory with the last applied resources IDs.

### Preview changes

After editing a bundle file, you can review the changes that will
be made on the cluster with `timoni bundle apply --diff`.

Example:

```shell
timoni bundle apply --dry-run --diff -f bundle.cue
```

### Uninstall

To uninstall the instances defined in a Bundle file,
you can use the `timoni bundle delete` command.

Example:

```shell
timoni bundle delete -f bundle.cue
```

### Garbage collection

Timoni's garbage collector keeps track of the applied resources and prunes the Kubernetes
objects that were previously applied but are missing from the current revision.

Example:

```shell
timoni bundle apply --prune -f bundle.cue
```

To prevent the garbage collector from deleting certain
resources such as Kubernetes Persistent Volumes,
these resources can be annotated with `action.timoni.sh/prune: "disabled"`.

The garbage collection is enabled by default, to opt-out set `--prune=false`.

### Readiness checks

By default, Timoni applies the instances in order, and will wait for
each instance's resources to become ready, before moving to the next instance.

The readiness check is performed for the Kubernetes resources with the following types:

- Kubernetes built-in kinds: Deployment, DaemonSet, StatefulSet,
  PersistentVolumeClaim, Pod, PodDisruptionBudget, Job, CronJob, Service,
  Secret, ConfigMap, CustomResourceDefinition
- Custom resources that are compatible with [kstatus](https://github.com/kubernetes-sigs/cli-utils/tree/master/pkg/kstatus)

Example:

```shell
timoni bundle apply --wait --timeout=5m -f bundle.cue
```

With `--timeout`, Timoni will retry the readiness checks until the specified
timeout period expires. If an instance's resource fails to become ready,
the apply command will exit with an error.

The readiness check is enabled by default, to opt-out set `--wait=false`.

### Linting

To verify that one or more CUE files contain a valid Bundle definition,
you can use the `timoni bundle lint` command.

Example:

```shell
timoni bundle lint -f bundle.cue -f extras.cue
```

To format Bundle files, you can use the `cue fmt` command.

Example:

```shell
cue fmt bundle.cue
```
