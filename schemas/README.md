# Timoni CUE Schemas

The Timoni's CUE schemas are included in the modules generated with `timoni mod init`.

## Schemas

### Kubernetes Metadata

- `#Annotations` - Schema for validating Kubernetes annotations.
- `Action` - Values of `action.timoni.sh` annotations for controlling Timoni's apply behaviour.
- `#Labels` - Schema for validating Kubernetes labels.
- `#StdLabel` - Values of Kubernetes `app.kubernetes.io` labels.
- `#Metadata` - Schema for generating Kubernetes metadata (name, namespace, annotations and labels),
  with automatic generation of `app.kubernetes.io/managed-by`,
  `app.kubernetes.io/name` and `app.kubernetes.io/version` labels.
- `#MetaComponent` - Superset of `#Metadata` schema for Kubernetes namespaced objects,
  that appends the component name to `metadata.name`, and
  with automatic generation of  `app.kubernetes.io/component` label.
- `#MetaClusterComponent` - Superset of `#Metadata` schema for Kubernetes global objects,
  that appends the component name to `metadata.name`, and
  with automatic generation of  `app.kubernetes.io/component` label.
- `#Selector` - Schema for generating Kubernetes label selectors
  with automatic generation of `app.kubernetes.io/name` label.

### Container Image

- `#Image` - Schema for generating a container image and pull policy
  from `repository`, `tag` and `digest` inputs.
- `#ImagePullSecret` - Schema for generating a Kubernetes Secret
  of type `kubernetes.io/dockerconfigjson`
  from `repository`, `username` and `password` inputs.

### Container Resource Requirements

- `#CPUQuantity` - Schema for validating Kubernetes CPU resource requirements.
- `#MemoryQuantity` - Schema for validating Kubernetes memory resource requirements.
- `#ResourceRequirements` - Schema for generating container requests and limits
  from `cpu` and `memory` inputs.

### Semantic Versioning

- `#SemVer` - Schema for validating semantic versions and enforcing
  a minimum major and/or minor version.

## Vendoring

To update a module's schemas to the latest version,
run the following command from within the module root:

```shell
timoni artifact pull oci://ghcr.io/stefanprodan/timoni/schemas:latest \
  --output cue.mod/pkg
```

To update the schemas and verify the signature with Cosign v2:

```shell
timoni artifact pull oci://ghcr.io/stefanprodan/timoni/schemas:latest \
  --verify=cosign \
  --certificate-identity-regexp="^https://github.com/stefanprodan/timoni.*$" \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com \
  --output cue.mod/pkg
```
