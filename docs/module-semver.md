# Module Versioning

Timoni modules are distributed as OCI artifacts. When publishing a module version
to a container registry, the version number is used as the OCI artifact tag.
The version number is also stored in the artifact manifest as the value of the
`org.opencontainers.image.revision` annotation.

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
  --source='https://github.com/my-org/my-app' \
  --latest=true \
  --version=1.0.0
```

To add custom metadata to a version, such as licenses and documentation links
please the [timoni mod push docs](cmd/timoni_mod_pull.md).

Example of publishing a pre-release version:

```shell
timoni mod push ./modules/my-app oci://ghcr.io/my-org/modules/my-app \
  --source='https://github.com/my-org/my-app' \
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

## Listing module versions

Timoni offer a command for listing all the versions available in a container registry for a particular module.

The `timoni mod list oci://<module-url>` prints a table with the versions order by semver
and the OCI digest corresponding to each version.

Example:

```console
$ timoni mod list oci://ghcr.io/stefanprodan/modules/redis
VERSION DIGEST                                                                  
latest  sha256:7fcb6f6918902c0dedc2ba4545fbdeca119a04644a53400af15b977e3921c600 
7.0.10  sha256:7fcb6f6918902c0dedc2ba4545fbdeca119a04644a53400af15b977e3921c600 
7.0.9   sha256:e9137d41b0d263bfaf2a43fc862648ad9dc3a976b4b0fc6e27617ea28ee27d45 
7.0.8   sha256:9340d4651f15305e7932c2fd9abf131d88fe77be2f0f3b6a0c3ede9772c3f622 
7.0.7   sha256:ba83770ce982fb6532a765009ed0429c4729bdeaa7421277618d4fb38c106be8 
```
