# Module Specification

A Timoni module describes how an application can be customised and deployed on Kubernetes,
using the CUE configuration language.

A module is composed of a set of Kubernetes objects and a well-defined configuration schema.
Besides the Kubernetes templates and config schema, a module can specify how the lifecycle
of the application should be managed, how the application should be tested.

The Timoni modules are packaged as Open Container Initiative (OCI)
artifacts and distributed through OCI registries.
Modules are semantically versioned and can be referenced by their OCI tag or digest.
A module version can be cryptographically signed when pushed to an OCI registry.

To facilitate the development, testing, and publishing of modules,
the Timoni CLI provides familiar commands tailored for these purposes.

## Module Structure

The Timoni module file structure consists of a set of directories and CUE files
organised into a CUE module.

Example of a module's root directory:

```sh
├── cue.mod
│   ├── gen # Kubernetes APIs and CRDs schemas
│   ├── pkg # Timoni APIs schemas
│   └── module.cue # Module metadata
├── templates
│   ├── config.cue # Config schema and default values
│   ├── deployment.cue # Kubernetes Deployment template
│   └── service.cue # Kubernetes Service template
├── timoni.cue # Timoni entry point
├── timoni.ignore # Timoni ignore rules
├── values.cue # Timoni values placeholder 
├── LICENSE # Module license
└── README.md # Module documentation
```

Timoni streamlines the creation of new modules through a blueprint-based approach.
In addition to utilizing official blueprints, users can develop and
employ custom blueprints for generating modules.
Timoni further assists in vendoring and keeping Kubernetes APIs and CRDs schemas
up-to-date.

## Module Artifacts

A module is packaged as an OCI artifact that contains the contents of the module
directory compressed as a gzipped tarball.
To optimize the registry storage and to minimize the bandwidth usage,
the vendored CUE definitions are stored in a separate artifact layer and 
cached locally.

The module version is reflected in the artifact tag,
and its metadata is stored in the artifact annotations.
By default, the module metadata includes the standard
`org.opencontainers.image` annotations and can be extended with custom annotations.
The metadata contains information about the module's source code
repository and revision, the module's author, license and documentation.

Example of a module's OCI manifest:

```json
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "config": {
    "mediaType": "application/vnd.timoni.config.v1+json",
    "size": 342,
    "digest": "sha256:62b6006cf2a56d7cfc8287164b25e47d9bc17b282aeb0701535897960ee13cbf"
  },
  "layers": [
    {
      "mediaType": "application/vnd.timoni.content.v1.tar+gzip",
      "size": 121873,
      "digest": "sha256:fff54591c495da1b19c647da5946c2e93c309e1389b3ac1a17ed293f2bb539fc",
      "annotations": {
        "sh.timoni.content.type": "module/vendor"
      }
    },
    {
      "mediaType": "application/vnd.timoni.content.v1.tar+gzip",
      "size": 7837,
      "digest": "sha256:e6af46874ac539bb63ac6ccf269bbc520c1e6ce5bc7180934cfa17b8cfc5a1b1",
      "annotations": {
        "sh.timoni.content.type": "module"
      }
    }
  ],
  "annotations": {
    "org.opencontainers.image.created": "2023-12-30T22:19:40Z",
    "org.opencontainers.image.description": "Get started blueprint for timoni.sh modules.",
    "org.opencontainers.image.documentation": "https://timoni.sh",
    "org.opencontainers.image.licenses": "Apache-2.0",
    "org.opencontainers.image.revision": "7a0566468fa4d49e3be269ad16f447c06e47de63",
    "org.opencontainers.image.source": "https://github.com/stefanprodan/timoni",
    "org.opencontainers.image.version": "0.19.0-rc.1"
  }
}
```

## Module Instance

A Timoni instance represent a module instantiation on a Kubernetes cluster.
The instance holds the inventory of the Kubernetes objects deployed on the cluster,
the OCI reference of the module, and the user-supplied configuration values.

In the cluster, the instance is stored as a Kubernetes Secret of type `timoni.sh/instance`.
The secret's name contains the instance name in the format `timoni.<instance name>`,
and the secret's namespace is the instance namespace.

The role of the instance Kubernetes Secret is to keep track of the managed objects and to
help Timoni's garbage collector to delete the objects when the instance is uninstalled.

Timoni offers a set of commands for inspecting the instance state:

- `timoni inspect module` - displays the module URL, digest, and version
- `timoni inspect values` - displays the instance config values
- `timoni inspect resources` - displays the Kubernetes objects managed by the instance

## Module Development

For an overview of CUE and the reasons why we chose it as the configuration language for Timoni,
please see the [CUE introduction](cue/introduction.md).

To get familiar with the CUE syntax, please see the
[CUE guide](cue/walkthrough.md). This guide will walk you through
the core features of CUE and how to use them to generate Kubernetes objects.

To get started with Timoni module development, please see the following guides:

- [Module initialization](cue/module/initialization.md)
- [Add Kubernetes custom resources](cue/module/custom-resources.md)
- [Cluster version constraints](cue/module/semver-constraints.md)
- [Control the Apply Behavior](cue/module/apply-behavior.md)
- [Run tests with Kubernetes Jobs](cue/module/test-jobs.md)

## Module Distribution

Timoni modules are distributed as OCI artifacts, for more information please see:

- [Module OCI artifact format](cue/module/publishing.md#artifact-format)
- [Module versioning](cue/module/publishing.md#version-format)
- [Publishing module versions](cue/module/publishing.md#publishing-module-versions)
- [Module signing and verification](cue/module/signing.md)
- [Module distribution with GitHub Actions](cue/module/github-actions.md)
