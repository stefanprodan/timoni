# Module Distribution

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

To enable reproducible builds, Timoni tries to determine the module's
last modified date, the source URL and source revision from the Git metadata.

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

## Publishing module versions

Timoni offers a command for publishing a module version
by packaging a local module as an OCI artifact and pushing
the artifact to a container registry.

When publishing a module with [timoni mod push](cmd/timoni_mod_push.md),
the version number must be specified using the `--version` flag.

Example of publishing version `1.0.0` as the latest stable release:

```shell
timoni mod push ./modules/my-app oci://ghcr.io/my-org/modules/my-app \
  --latest=true \
  --version=1.0.0
```

To add custom metadata to a version, such as licenses and documentation links
please the [timoni mod push docs](cmd/timoni_mod_pull.md).

Example of publishing a pre-release version:

```shell
timoni mod push ./modules/my-app oci://ghcr.io/my-org/modules/my-app \
  --latest=false \
  --version=2.0.0-beta.1
```

The `--latest` flag is used to mark a stable release as the latest version.
When `--latest` is set to `true`, Timoni will push the version tag,
then it will tag the version as `latest` in the container registry.

!!! tip "Immutability"

    Note that a Timoni module version should be immutable.
    The only tag that is suppose to be mutable, is the `latest` one.
    Overriding an existing version tag in the container registry should be avoided,
    some registries allow enforcing immutability for semver tags but this is not guranteed by default.

To automate the publishing of module versions, please see the [Timoni GitHub Actions](github-actions.md).

To cryptographically sign a module version, please see the [Timoni module signing and verification doc](module-sign.md).

## Listing module versions

Timoni offer a command for listing all the versions available in a container registry for a particular module.

The `timoni mod list oci://<module-url>` prints a table with the versions order by semver
and the OCI digest corresponding to each version.

Example:

```console
$ timoni mod list oci://ghcr.io/stefanprodan/modules/podinfo
VERSION	DIGEST                                                                  
latest  sha256:d5cb5a8c625045ee1da01d629a2d46cd361f2b6472b8bd07bcabbd0012bc574b 
6.5.0   sha256:d5cb5a8c625045ee1da01d629a2d46cd361f2b6472b8bd07bcabbd0012bc574b 
6.4.1   sha256:b3ef5d3ce9f0246e2c596db62f2d0830f58aa2b7f07199fe002c913d991e9d0c 
6.4.0   sha256:f2d85995c0936221eb58a16ead23125d81154ab2cd25a31b2b6314f1f584ee0b 
```
