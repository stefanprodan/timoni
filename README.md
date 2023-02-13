# timoni

Timoni is a package manager for Kubernetes powered by [CUE](https://github.com/cue-lang/cue)
and inspired by [Helm](https://github.com/helm/helm).

Timoni is for people that want to build and distribute applications to Kubernetes
without having to write a single line of YAML. Authoring Kubernetes configs can be
a more pleasant experience with CUE than mingling Go templates with YAML.

> **Warning**
> Note that Timoni is in the experimental phase, you need be familiar with CUE in order to author modules.

## Timoni vs Helm

Timoni is akin to Helm's user experience but not to the authoring experience.

Authoring differences:

- Instead of charts, Timoni works with CUE modules which are distributed as OCI artifacts.
- Instead of Go templating of Kubernetes YAML, Timoni works with CUE templates and CUE types generated from Kubernetes API Go types.
- Instead of relying on 3rd-party tool to validate custom resources, Timoni uses the CUE types generated from Kubernetes CRDs.

User differences:

- Timoni modules, the equivalent of Helm charts, can only be pushed and pulled to/from a container registry which supports custom OCI media types.
- Instead of supplying values in YAML format, Timoni expects users to supply values as CUE definitions.
- Instead of Helm's client-side apply, Timoni uses Kubernetes server-side apply and [Flux](https://fluxcd.io)'s drift detection.
