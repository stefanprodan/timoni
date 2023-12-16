# Bundle Distribution

[Bundles](bundle.md) and their [Runtimes](bundle-runtime.md) can be distributed as
[Open Container Initiative](https://opencontainers.org/) (OCI) artifacts.

## Artifact format

The OCI artifacts produced with `timoni artifact push` have the following media types:

- Image media type `application/vnd.oci.image.manifest.v1+json`
- Config media type `application/vnd.timoni.config.v1+json`
- Layer media type `application/vnd.timoni.content.v1.tar+gzip`

The artifacts are annotated with OCI
[standard annotations](https://specs.opencontainers.org/image-spec/annotations/?v=v1.0.1#pre-defined-annotation-keys):

- `org.opencontainers.image.source: <GIT URL>`
- `org.opencontainers.image.revision: <GIT COMMIT SHA>`
- `org.opencontainers.image.created: <GIT COMMIT DATE>`

To enable reproducible builds, Timoni tries to determine the 
source, revision and created date from the Git metadata.

## Publishing bundles to container registries

Timoni offers a command for publishing bundles, by packaging a local directory
as an OCI artifact and pushing the artifact to a container registry.

When publishing bundles with [timoni artifact push](cmd/timoni_artifact_push.md) you can:

- specify a local directory contain any number of files and subdirectories using the `-f` flag
- choose a versioning scheme for your bundles using the `--tag` flag
- attach custom metadata such as licenses, Git source and documentation links using the `--annotation` flag
- sign the artifact using either with a [Cosign](https://github.com/sigstore/cosign) static key or Cosign keyless using the `--sign` flag

### Push and sign example

Login to the container registry:

```shell
echo $DOCKER_TOKEN | timoni registry login docker.io -u timoni --password-stdin
```

Generate a cosign key pair:

```shell
cosign generate-key-pair
```

Export the cosign private key password with:

```shell
export COSIGN_PASSWORD=<your password>
```

Publish and sign an artifact containing bundles and runtimes files:

```shell
timoni artifact push oci://docker.io/my-org/my-app-bundle \
  -f ./my-app/bundles/ \
  --sign=cosign \
  --cosign-key=cosign.key \
  --tag=1.0.0 \
  --tag=latest \
  --annotation='org.opencontainers.image.licenses=Apache-2.0' \
  --annotation='org.opencontainers.image.documentation=https://app.org/docs' \
  --annotation='org.opencontainers.image.source=https://github.com/my-org/my-app'
```

Add extra tags to the artifact with:

```shell
timoni artifact tag oci://docker.io/my-org/my-app-bundle:1.0.0 \
  --tag=1.0 \
  --tag=1
```

## Using bundles from container registries

Timoni offers commands for listing, verifying and extracting bundles
and any other files from OCI artifacts stored in container registries.

### List, pull and apply example

If the artifact is stored in a private registry, login with:

```shell
echo $DOCKER_TOKEN | timoni registry login docker.io -u timoni --password-stdin
```

List all the artifact's tags and digest with [timoni artifact list](cmd/timoni_artifact_list.md):

```shell
timoni artifact list oci://docker.io/my-org/my-app-bundle
```

Verify the signature and download a specific artifact tag with [timoni artifact pull](cmd/timoni_artifact_pull.md):

```shell
timoni artifact pull oci://docker.io/org/app:latest \
  --verify cosign \
  --cosign-key cosign.pub \
  --output .
```

If the artifact was signed using Cosign keyless, you can verify it with:

```shell
timoni artifact pull oci://docker.io/org/app:latest \
  --verify cosign \
  --certificate-identity-regexp="^https://github.com/org/.*$" \
  --certificate-oidc-issuer-regex="^https://token.actions.githubusercontent.com.*$"
```

The above command will extract all the files from the remote artifact into the current directory.

Deploy the app from a bundle and runtime with [timoni bundle apply](cmd/timoni_bundle_apply.md):

```shell
timoni bundle apply \
  -f app.bundle.cue \
  --runtime app.runtime.cue \
  --kube-context my-test-cluster
```
