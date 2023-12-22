# Concepts

![Timoni](images/logo_text_black.svg#only-light){ width="200" }
![Timoni](images/logo_text_white.svg#only-dark){ width="200" }

- [Module](#module) - App definition containing Kubernetes CUE templates and configuration schema, distributed as OCI artifacts.
- [Instance](#instance) - App instantiation referencing the module and workloads deployed on a Kubernetes cluster.
- [Bundle](#bundle) - App composition bundling multiple modules and configurations into a deployable unit.
- [Artifact](#artifact) - Packaging format used for distributing modules and bundles to container registries.

## :fontawesome-solid-cube: Module

A Timoni module contains a set of CUE definitions and constraints organised
into a [CUE module](https://cuelang.org/docs/concepts/packages/)
with an opinionated structure.

A module accepts a set of values supplied by users,
and outputs a set of Kubernetes objects that Timoni deploys on Kubernetes clusters.

Module structure:
```sh
app
├── cue.mod # Kubernetes APIs and CRDs schemas
├── templates # Workloads and app config schema
├── timoni.cue # Timoni entry point
└── values.cue # Default config values
```

Commands for working with local modules:

- `timoni mod init <module-name>`
- `timoni mod vet <path/to/module>`
- `timoni build <name> <path/to/module> -n <namespace>`
- `timoni apply <name> <path/to/module> -f <path/to/values.cue> --dry-run --diff`

Commands for vendoring Kubernetes APIs and CRDs:

- `timoni mod vendor k8s --version latest`
- `timoni mod vendor crds -f <path/to/crds.yaml>`

Timoni modules are distributed as OCI artifacts that can be cryptographically [signed and verified](module-sign.md).
Modules are versioned using strict [semantic versioning](module-distribution.md#version-format),
the version of a module is used as the OCI artifact tag.

To learn more about modules, please see the [module documentation](module.md).

## :fontawesome-solid-share-from-square: Instance

A Timoni instance represent a module instantiation on a Kubernetes cluster.
A module can be installed multiple times on a cluster by giving its instances
unique names per namespace.

When instantiating a module, users can supply their own `values.cue`
that gets merged with the defaults included in the module:

```cue
values: {
	ingress: {
		enabled:   true
		className: "nginx"
		host:      "app.example.com"
	}
	autoscaling: enabled: true
	monitoring: enabled:  true
}
```

Commands for working with module instances:

- `timoni install <name> oci://<module-url> -v <semver> -n <namespace>`
- `timoni upgrade <name> oci://<module-url> -v <semver> -f <path/to/values.cue>`
- `timoni uninstall <name> -n <namespace>`
- `timoni list -n <namespace>`
- `timoni inspect [module|values|resources] <name> -n <namespace>`
- `timoni status <name> -n <namespace>`

The `install` and `upgrade` commands are aliases of `timoni apply`.
To apply the Kubernetes resources belonging to a module instance,
Timoni uses Kubernetes server-side apply and
[Flux](https://fluxcd.io)'s drift detection.

The apply command validates all resources with a dry-run apply,
and reconciles only the ones with changes to the cluster state.

Timoni's garbage collector keeps track of the applied resources
and prunes the Kubernetes objects that were previously applied
but are missing from the current revision.

After an installation or upgrade, Timoni waits for the
applied resources to be fully reconciled by checking the ready status
of deployments, jobs, services, ingresses, and Kubernetes custom resources.

## :fontawesome-solid-layer-group: Bundle

Timoni bundles offer a declarative way of managing the lifecycle of applications and their infra dependencies.

A Timoni bundle is a CUE file for defining a group of instances together with their values and module references:

```cue
bundle: {
	apiVersion: "v1alpha1"
	name:       "podinfo"
	instances: {
		redis: {
			module: {
				url:     "oci://ghcr.io/stefanprodan/modules/redis"
				version: "7.0.9"
			}
			namespace: "podinfo"
			values: maxmemory: 256
		}
		podinfo: {
			module: url:     "oci://ghcr.io/stefanprodan/modules/podinfo"
			module: version: "6.3.4"
			namespace: "podinfo"
			values: caching: {
				enabled:  true
				redisURL: "tcp://redis:6379"
			}
		}
	}
}
```

For deterministic operations, it is possible to pin a module version by its OCI digest:

```cue
redis: {
	module: {
		url:    "oci://ghcr.io/stefanprodan/modules/redis"
		digest: "sha256:e9137d41b0d263bfaf2a43fc862648ad9dc3a976b4b0fc6e27617ea28ee27d45"
	}
}
```

!!! tip "Bundle example"

    An example bundle can be found in Timoni's repository at
    [examples/bundles/podinfo.cue](https://github.com/stefanprodan/timoni/tree/main/examples/bundles/).
    This bundle defines a Redis master-replica cluster and a podinfo instance connected to the Redis instance.
    The secret values are defined in a separate file which can be kept encrypted or pulled from a secure vault at apply time.

In the bundle files you can use arithmetic operations,
string interpolation and everything else that CUE std lib supports.

Commands for working with bundles:

- `timoni bundle apply -f bundle.cue --runtime runtime.cue --diff`
- `timoni bundle build -f bundle.cue -f bundle_extras.cue`
- `timoni bundle delete -f bundle.cue`
- `timoni bundle vet -f bundle.cue`

To learn more about bundles, please see the [Bundle API documentation](bundle.md)
and the [Bundle Runtime API documentation](bundle-runtime.md).

## :fontawesome-solid-box: Artifact

Timoni modules and bundles are distributed to container registries as
[Open Container Initiative](https://opencontainers.org/) (OCI) artifacts.

The OCI artifacts produced by Timoni have the following media types:

- Image media type `application/vnd.oci.image.manifest.v1+json`
- Config media type `application/vnd.timoni.config.v1+json`
- Layer media type `application/vnd.timoni.content.v1.tar+gzip`

To enable reproducible builds, Timoni sets the artifact's last modified date,
the source URL and source revision annotations from the Git metadata.

Commands for managing the authentication to container registries:

- `timoni registry login <registy-host> -u <user> -p <password>`
- `timoni registry logout <registy-host>`

Commands for distributing modules:

- `timoni mod push <path/to/module> oci://<module-url> -v <semver> --sign`
- `timoni mod pull oci://<module-url> -v <semver> -o <path/to/module> --verify`
- `timoni mod list oci://<module-url>`

Commands for distributing bundles and runtimes:

- `timoni artifact push oci://<artifact-url> -t <tag> -f <path/to/dir>`
- `timoni artifact pull oci://<artifact-url> -o <path/to/dir>`
- `timoni artifact list oci://<artifact-url>`
- `timoni artifact tag oci://<artifact-url>:<tag> -t <new-tag>`
