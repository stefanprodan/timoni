# Timoni vs other tools

## Helm

If you are familiar with Helm, a Timoni **[module](bundles.md)** is the equivalent of a **chart**, 
a Timoni **[bundle](bundles.md)** is the equivalent of an **umbrella chart**,
and a Timoni **instance** is the equivalent of a Helm **release**.

### Authoring differences

- Instead of using charts, Timoni utilizes CUE modules distributed as OCI artifacts.
- Timoni modules can only be pushed and pulled to/from container registries.
- Timoni modules can be signed and verified with Cosign for integrity, unlike Helm's reliance on Helm provenance and OpenPGP signatures.
- Timoni works with CUE templates and CUE types generated from Kubernetes API Go types, eliminating the need for Go templating of Kubernetes YAML.
- Timoni leverages CUE types generated from Kubernetes CRDs for validating custom resources, eliminating the need for 3rd-party tooling like kubeconform.
- Timoni allows authors to group Kubernetes resources and define the apply order for each group, instead of using Helm's pre/post install/upgrade hooks.

### User differences

- Timoni expects users to provide values as CUE definitions instead of supplying them in YAML format.
- Unlike Helm, Timoni allows condition statements in values and bundle files, enabling users to express logic using CUE.
- Timoni employs Kubernetes [server-side apply](https://kubernetes.io/docs/reference/using-api/server-side-apply/) and [Flux](https://fluxcd.io)'s drift detection instead of Helm's client-side apply.
- Timoni patches only the Kubernetes objects that have changed during upgrades, while Helm applies all manifests.
- Timoni applies resources in stages, ensuring readiness for each resource group before proceeding to the next one (e.g., CRDs and namespaces, then workloads, then custom resources).
- Timoni supports upgrading CRDs and their controllers, unlike Helm, which ignores changes to CRDs.
- Timoni enables templating of CRDs, whereas Helm only supports plain YAML CRDs.
- Timoni's garbage collector can delete CRDs and PVCs, whereas Helm leaves them on the cluster during uninstallation.
- Timoni performs health checks on Kubernetes custom resources, which Helm ignores.
- Unlike Helm, Timoni doesn't require keeping a copy of all YAML manifests in a Kubernetes secret, eliminating the limit on the number of objects constituting an application.
- Timoni provides a `apply --diff` command for displaying a preview of the cluster state changes for an upgrade.
- Timoni bundles offer a declarative way of grouping multiple apps into a deployable unit, serving as an alternative to Helm's umbrella charts.
- Timoni modules can be referenced by their OCI SHA256 digest, ensuring immutability and reproducibility, unlike Helm charts that are referenced only by version.
