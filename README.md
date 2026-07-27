# timoni

[![Sponsor this](https://img.shields.io/static/v1?label=Sponsor&message=%E2%9D%A4&logo=GitHub&link=https://github.com/sponsors/stefanprodan)](https://github.com/sponsors/stefanprodan)
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

> [!IMPORTANT]
> Note that Timoni in under active development and is still in its infancy.
> The APIs and command-line interface may change in a backwards incompatible manner.

## Get Started

To get started with Timoni please visit the documentation website at [timoni.sh](https://timoni.sh/quickstart/).

## Concepts

- [Module](https://timoni.sh/concepts/#module) - App definition containing Kubernetes CUE templates and configuration schema, distributed as OCI artifacts.
- [Instance](https://timoni.sh/concepts/#instance) - App instantiation referencing the module and workloads deployed on a Kubernetes cluster.
- [Bundle](https://timoni.sh/concepts/#bundle) - App composition bundling multiple modules and configurations into a deployable unit.
- [OCI Artifact](https://timoni.sh/concepts/#artifact) - Packaging format used for distributing modules and bundles through registries or local OCI archives and layouts.

> [!TIP]
> If you are familiar with Helm, a Timoni **[module](https://timoni.sh/module/)** is the equivalent of a **chart**,
> a Timoni **[bundle](https://timoni.sh/bundle/)** is the equivalent of an **umbrella chart**,
> and a Timoni **[instance](https://timoni.sh/concepts/#instance)** is the equivalent of a Helm **release**.

## Features

### App Packaging and Distribution

Timoni enables software vendors to define complex application deployments,
packaged as [Modules](https://timoni.sh/module/), using type-safe
Kubernetes templates and rich customisation options for end-users.

The app configuration packaged in a Module is
[distributed](https://timoni.sh/cue/module/publishing/) as an
Open Container Initiative (OCI) artifact, next to the app images,
in a container registry. Timoni Modules are semantically versioned
and can be cryptographically [signed](https://timoni.sh/cue/module/signing/) when pushed.
Local build outputs are unsigned OCI archives or layouts for offline handoff.

Module distribution:

```shell
timoni mod push ./my-app -v 1.0.0 oci://registry.example.com/my-app
timoni mod build ./my-app -v 1.0.0 -o ./my-app-1.0.0.oci.tar
```

Generic artifacts transport bundle files, runtime files, and other content without module, instance, or deployment semantics:

```shell
timoni artifact push -f ./delivery -t 1.0.0 oci://registry.example.com/my-app
timoni artifact build -f ./delivery -t 1.0.0 -o ./my-app-delivery-1.0.0.oci.tar
```

The timoni CLI consumes module artifacts; Kubernetes nodes consume separate workload images. Local OCI outputs are for transport, not `apply` inputs; use a module directory for registry-free apply.

With Timoni, platform engineers can manage the lifecycle of Kubernetes
controllers, including the upgrade of CRDs. Module authors can
[import CRD schemas](https://timoni.sh/cue/module/custom-resources/)
from YAML files and incorporate Kubernetes custom resources
in their app deployments.

### App Lifecycle Management

With Timoni, users can manage the whole lifecycle of applications deployed on Kubernetes.
From highly customised installation to seamless upgrades,
end-to-end testing, safe rollback and uninstallation.

With Timoni, users can bundle microservices and distributed monoliths into a deployable unit.
The Timoni [Bundle](https://timoni.sh/bundle/) offers a declarative way of managing
the app delivery across clusters, where secrets and other environment-specific config
values are [dynamically loaded](https://timoni.sh/bundle-runtime/) during installation or upgrades.

Bundle lifecycle:

```shell
timoni bundle build  -f ./bundle.cue # Render without applying (optional)
timoni bundle apply  -f ./bundle.cue # Install or upgrade
timoni bundle delete -f ./bundle.cue # Uninstall
```

## License

Timoni is [Apache 2.0 licensed](LICENSE) and accepts contributions via GitHub pull requests.
Please see the [contributing guide](CONTRIBUTING.md) for more information.
