# timoni

[![release](https://img.shields.io/github/release/stefanprodan/timoni/all.svg)](https://github.com/stefanprodan/timoni/releases)
[![build](https://github.com/stefanprodan/timoni/workflows/build/badge.svg)](https://github.com/stefanprodan/timoni/actions)
[![e2e](https://github.com/stefanprodan/timoni/workflows/e2e/badge.svg)](https://github.com/stefanprodan/timoni/actions)
[![license](https://img.shields.io/github/license/stefanprodan/timoni.svg)](https://github.com/stefanprodan/timoni/blob/main/LICENSE)

[Timoni](https://timoni.sh) is a package manager for Kubernetes,
powered by [CUE](https://cuelang.org/)
and inspired by [Helm](https://helm.sh/).

The Timoni project strives to improve the UX of authoring Kubernetes configs.
Instead of mingling Go templates with YAML like Helm,
or layering YAML on top of each-other like Kustomize,
Timoni relies on cuelang's type safety, code generation and data validation features
to offer a better experience of creating, packaging and delivering apps to Kubernetes.

> **Warning**
>
> Note that Timoni in under active development and is still in its infancy.
> Its APIs and interfaces may change in a backwards incompatible manner.

## Get started

To get started with Timoni please visit the documentation website at [timoni.sh](https://timoni.sh/).

## Concepts

If you are familiar with Helm, a Timoni **module** is the equivalent of a **chart**,
and a Timoni **instance** is the equivalent of a Helm **release**.

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
│   └── module.cue
├── templates # Kubernetes definitions
│   ├── config.cue
│   ├── deployment.cue
│   └── service.cue
├── timoni.cue # Timoni entry point
└── values.cue # Default values
```

A module example can be found at [examples/podinfo](examples/podinfo).

Commands for working with local modules:

- `timoni lint <path/to/module>`
- `timoni build <name> <path/to/module> -n <namespace>`
- `timoni apply <name> <path/to/module> -f <path/to/values.cue> --dry-run --diff`

Timoni modules are distributed as OCI artifacts and can be stored in container registries.

Commands for working with remote modules:

- `timoni push <path/to/module> oci://<module-url> -v <semver>`
- `timoni pull oci://<module-url> -v <semver> -o <path/to/module>`

To learn more about modules, please read the [docs](https://timoni.sh/#timoni-modules).

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

Commands for working with instances:

- `timoni install <name> oci://<module-url> -v <semver> -n <namespace>`
- `timoni upgrade <name> oci://<module-url> -v <semver> -f <path/to/values.cue>`
- `timoni uninstall <name> -n <namespace>`
- `timoni list -n <namespace>`
- `timoni inspect [module|values|resources] <name>`

To learn more about instances, please read the [docs](https://timoni.sh/#timoni-instances).

## Contributing

Timoni is [Apache 2.0 licensed](LICENSE) and accepts contributions via GitHub pull requests.
Please see the [contributing guide](CONTRIBUTING.md) for more information.
