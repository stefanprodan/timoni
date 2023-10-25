# timoni

[![release](https://img.shields.io/github/release/stefanprodan/timoni/all.svg)](https://github.com/stefanprodan/timoni/releases)
[![platforms](https://img.shields.io/badge/platforms-linux|macos|windows-9cf.svg)](https://timoni.sh/install)
[![build](https://github.com/stefanprodan/timoni/workflows/build/badge.svg)](https://github.com/stefanprodan/timoni/actions)
[![license](https://img.shields.io/github/license/stefanprodan/timoni.svg)](https://github.com/stefanprodan/timoni/blob/main/LICENSE)
[![SLSA 3](https://slsa.dev/images/gh-badge-level3.svg)](https://slsa.dev)

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
> The APIs and command-line interface may change in a backwards incompatible manner.

## Get started

To get started with Timoni please visit the documentation website at [timoni.sh](https://timoni.sh/).

## Concepts

- [Module](https://timoni.sh/concepts/#module) - App definition containing Kubernetes CUE templates and configuration schema, distributed as OCI artifacts.
- [Instance](https://timoni.sh/concepts/#instance) - App instantiation referencing the module and workloads deployed on a Kubernetes cluster.
- [Bundle](https://timoni.sh/concepts/#bundle) - App composition bundling multiple modules and configurations into a deployable unit.
- [OCI Artifact](https://timoni.sh/concepts/#artifact) - Packaging format used for distributing modules and bundles to container registries.

> **Note**
>
> If you are familiar with Helm, a Timoni **[module](https://timoni.sh/module/)** is the equivalent of a **chart**,
> a Timoni **[bundle](https://timoni.sh/bundle/)** is the equivalent of an **umbrella chart**,
> and a Timoni **[instance](https://timoni.sh/concepts/#instance)** is the equivalent of a Helm **release**.

## Contributing

Timoni is [Apache 2.0 licensed](LICENSE) and accepts contributions via GitHub pull requests.
Please see the [contributing guide](CONTRIBUTING.md) for more information.
