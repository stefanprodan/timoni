# Module Publishing

Timoni modules are distributed as [Open Container Initiative](https://opencontainers.org/)
(OCI) artifacts. When publishing a module version to a container registry,
the version number is used as the OCI artifact tag.

## Artifact format

The OCI artifacts produced with `timoni mod push` have the following media types:

- Image media type `application/vnd.oci.image.manifest.v1+json`
- Config media type `application/vnd.timoni.config.v1+json`
- Layer media type `application/vnd.timoni.content.v1.tar+gzip`

The artifacts are annotated with OCI
[standard annotations](https://specs.opencontainers.org/image-spec/annotations/?v=v1.0.1#pre-defined-annotation-keys):

- `org.opencontainers.image.version: <MODULE VERSION>`
- `org.opencontainers.image.created: <MODULE LAST MODIFIED DATE>`
- `org.opencontainers.image.source: <MODULE GIT URL>`
- `org.opencontainers.image.revision: <MODULE GIT SHA>`

For reproducible builds, Timoni preserves explicit creation, source and
revision annotations, then uses `SOURCE_DATE_EPOCH` or the Git commit time for
the creation date. Git source and revision metadata are added when available.

## Version format

The version format used by Timoni follows the [SemVer 2](https://semver.org/spec/v2.0.0.html)
standard, with the following restrictions:

- The version number must take the form `X.Y.Z` where X, Y, and Z are non-negative integers, and must not contain leading zeroes.
- Prefixing the version number with `v` is not supported.
- Suffixing the version number with build metadata by appending a plus sign is not supported, because the `+` sign can't be used in OCI tags.

The supported formats are:

- `0.Y.Z` - denotes initial development e.g. `0.1.2`
- `X.Y.Z` - denotes a stable release when  `X >= 1` e.g. `1.0.0` or `1.23.45`
- `X.Y.Z-alpha.N` - denotes an alpha pre-release e.g. `2.0.0-alpha.1`
- `X.Y.Z-beta.N` - denotes a beta pre-release e.g. `2.0.0-beta.1`
- `X.Y.Z-rc.N` - denotes a releases candidate e.g. `2.0.0-rc.1`

## Building modules without a registry

[timoni mod build](../../cmd/timoni_mod_build.md) writes the same ordered
vendor and module layers as `timoni mod push`, without registry or network
access:

```shell
timoni mod build ./modules/my-app -v 1.0.0 -o ./my-app-1.0.0.oci.tar
skopeo copy oci-archive:./my-app-1.0.0.oci.tar docker://ghcr.io/org/modules/app:1.0.0
```

The version is required and becomes the manifest version annotation and local
reference name. Use `--format=oci-layout` for directory output.
Local outputs are unsigned; existing output paths are rejected.

The archive produced by `mod build` can be pushed to a registry with
`timoni mod push` without re-packaging the source module. The version is read
from the archive manifest, so it must not be passed again:

```shell
timoni mod build ./modules/my-app -v 1.0.0 -o ./my-app-1.0.0.oci.tar
timoni mod push ./my-app-1.0.0.oci.tar oci://ghcr.io/org/modules/app
```

Annotations and symlink resolution are fixed at build time, so
`--annotation` and `--resolve-symlinks` are not supported when pushing a
pre-built archive.

## Publishing module versions

Timoni offers a command for publishing a module version
by packaging a local module as an OCI artifact and pushing
the artifact to a container registry.

When publishing a module with [timoni mod push](../../cmd/timoni_mod_push.md),
the version number must be specified using the `--version` flag.

### Stable versions

Example of publishing version `1.0.0` as the latest stable release:

```shell
timoni mod push ./modules/my-app oci://ghcr.io/my-org/modules/my-app \
  --latest=true \
  --version=1.0.0
```

To add custom metadata to a version, such as licenses and documentation links
please the [timoni mod push docs](../../cmd/timoni_mod_push.md).

### Pre-release versions

Example of publishing a pre-release version:

```shell
timoni mod push ./modules/my-app oci://ghcr.io/my-org/modules/my-app \
  --latest=false \
  --version=2.0.0-rc.1
```

### Latest version

The `--latest` flag is used to mark a stable release as the latest version.
When `--latest` is set to `true`, Timoni will push the version tag,
then it will tag the version as `latest` in the container registry.

!!! tip "Immutability"

    Note that a Timoni module version should be immutable.
    The only tag that is suppose to be mutable, is the `latest` one.
    Overriding an existing version tag in the container registry should be avoided,
    some registries allow enforcing immutability for semver tags but this is not guranteed by default.

To automate the publishing of module versions, please see the [Timoni GitHub Actions doc](github-actions.md).

### Ignoring files

Timoni modules can contain files that are not meant to be published.
To exclude files from the module artifact, you can add a `timoni.ignore` file
in the module root directory.

The `timoni.ignore` file must contain rules in the
[.gitignore pattern format](https://git-scm.com/docs/gitignore#_pattern_format).

It is recommended to exclude Git, Go and CUE tools related files, for example:

```.gitignore
# VCS
.git/
.gitignore
.gitmodules
.gitattributes

# Go
vendor/
go.mod
go.sum

# CUE
*_tool.cue
debug_values.cue
```

### Symbolic links

When packaging a module, Timoni skips symbolic links by default.
To include the link targets in the artifact as regular files and
directories, run `timoni mod push` with the `--resolve-symlinks` flag:

```shell
timoni mod push ./path/to/module oci://docker.io/org/app-module \
  --version=1.0.0 \
  --resolve-symlinks
```

This allows sharing files between modules in a monorepo, for example
by symlinking the vendored CUE schemas under `cue.mod/gen`.

The `timoni.ignore` rules apply to the resolved content at its
in-module path, the same as for regular files and directories.

The `timoni artifact push` command takes the same flag for packaging
the symlink targets found in the pushed directory.

!!! tip "Local modules"

    When building a module from a local path, `timoni build`, `timoni apply`
    and `timoni mod vet` read the module files in place from the source
    directory, following symbolic links the same way as the operating system.
    Note that a module which relies on symlinked files builds locally but
    is published incomplete unless pushed with `--resolve-symlinks`.

## Listing module versions

Timoni offers a command for listing all the versions available in a
container registry for a particular module.

The `timoni mod list oci://<module-url>` prints a table with the versions order
by semver and the OCI digest corresponding to each version.

Example:

```console
$ timoni mod list oci://ghcr.io/stefanprodan/modules/podinfo
VERSION DIGEST                                                                  
latest  sha256:ddf8be195f69ad767b40d5c3fec3a8a85482bf6bf469d949a462cb4fd27df580 
6.14.1  sha256:ddf8be195f69ad767b40d5c3fec3a8a85482bf6bf469d949a462cb4fd27df580 
6.14.0  sha256:32082e8ac0bba9ee2a3f95534f52c94f5a41dc642bb396c73b04fa18aff147d7 
6.13.0  sha256:37312e975dc9bc6c1f4ebccc90f55caf2d35ce15f1b0f59794348924fa0193e9 
6.12.0  sha256:99bdcbc42933bd0d14c0c5496541059063f0c111393f5b773cd0e2bbf6bed435 
6.11.2  sha256:12b1fa57fdf554110a1ec3519d02f69a6896515967d652029bc953a7e988ff29 
```
