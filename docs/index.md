# Timoni

[Timoni](https://github.com/stefanprodan/timoni) is a package manager for Kubernetes,
powered by [CUE](https://cuelang.org/)
and inspired by [Helm](https://helm.sh/).

The Timoni project strives to improve the UX of authoring Kubernetes configs.
Instead of mingling Go templates with YAML like Helm,
or layering YAML on top of each-other like Kustomize,
Timoni relies on cuelang's type safety, code generation and data validation features
to offer a better experience of creating, packaging and delivering apps to Kubernetes.

!!! warning "Development phase"

    Timoni in under active development and is still in its infancy.
    The APIs and interfaces may change in a backwards incompatible manner.

## Concepts

### Timoni Modules

A Timoni module contains a set of CUE definitions and constraints organised
into a [CUE module](https://cuelang.org/docs/concepts/packages/)
with an opinionated structure.
A module accepts a set of values supplied by the user as `values.cue` files,
and outputs a set of Kubernetes objects that Timoni deploys on Kubernetes.

Module structure:
```sh
├── cue.mod
│   ├── gen # Kubernetes types
│   └── module.cue # Module metadata
├── templates
│   ├── config.cue # Config schema and default values
│   ├── deployment.cue # Kubernetes Deployment template
│   └── service.cue # Kubernetes Service template
├── timoni.cue # Timoni entry point
└── values.cue # Timoni values placeholder
```

!!! tip "Module example"

    An example module can be found in Timoni's repository at
    [examples/podinfo](https://github.com/stefanprodan/timoni/tree/main/examples/podinfo).
    The example module is published to GitHub Container Registry and is publicly available at
    [ghcr.io/stefanprodan/modules/podinfo](https://github.com/stefanprodan/podinfo/pkgs/container/modules%2Fpodinfo).

Commands for working with local modules:

- `timoni mod init <module-name>`
- `timoni mod lint <path/to/module>`
- `timoni build <name> <path/to/module> -n <namespace>`
- `timoni apply <name> <path/to/module> -f <path/to/values.cue> --dry-run --diff`

#### OCI Artifacts

Timoni modules are distributed as OCI artifacts and can be stored in container registries
which support custom OCI media types.
Modules are versioned using strict [semantic versioning](https://semver.org/),
the version of a module is used as the OCI artifact tag.

Commands for working with remote modules:

- `timoni mod push <path/to/module> oci://<module-url> -v <semver>`
- `timoni mod pull oci://<module-url> -v <semver> -o <path/to/module>`
- `timoni mod list oci://<module-url>`

Timoni produces artifacts compatible with Docker Hub, GitHub Container Registry,
Azure Container Registry, Amazon Elastic Container Registry, Google Artifact Registry,
self-hosted Docker Registry and others.

### Timoni Instances

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

### Timoni Bundles

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
        url:     "oci://ghcr.io/stefanprodan/modules/redis"
        digest:  "sha256:e9137d41b0d263bfaf2a43fc862648ad9dc3a976b4b0fc6e27617ea28ee27d45"
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

- `timoni bundle lint -f bundle.cue`
- `timoni bundle apply -f bundle.cue -f bundle_extras.cue --dry-run --diff`
- `timoni bundle delete -f bundle.cue -f bundle_extras.cue`
- `timoni bundle build -f bundle.cue -f bundle_extras.cue`

To learn more about bundles, please see the [Bundle API documentation](https://timoni.sh/bundles/).

## License

Timoni is [Apache 2.0 licensed](https://github.com/stefanprodan/timoni/blob/main/LICENSE)
and accepts contributions via GitHub pull requests.
