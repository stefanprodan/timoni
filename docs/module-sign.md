# Module Signing and Verification

Timoni modules are distributed as OCI artifacts. When publishing a module version to a container registry,
the OCI artifact can be cryptographically signed to improve the software supply chain security.

## Cosign

Timoni can sign modules using Sigstore Cosign.
[Cosign](https://github.com/sigstore/cosign) is tool that allows you to sign and verify
OCI artifacts with a public/private key pair or with an OIDC token provided by GitHub, Google or Microsoft.

To sign modules, you need to [install](https://docs.sigstore.dev/system_config/installation/)
the Cosign v2 binary and place it in the `PATH` for Timoni to use it.

### Sign with static keys

Generate a cosign key pair:

```shell
cosign generate-key-pair
```

Export the private key password with:

```shell
export COSIGN_PASSWORD=<your password>
```

Sign the module while pushing:

```shell
timoni mod push ./modules/my-app oci://ghcr.io/my-org/modules/my-app \
  --version=1.0.0 \
  --sign=cosign \
  --cosign-key=cosign.key
```

Timoni will push the module to the registry and will pass the OCI artifact digest to Cosign.
Cosign will push the signature to the registry and will record the signature in
the [Rekor](https://github.com/sigstore/rekor) transparency log.

To verify the module signature:

```shell
cosign verify ghcr.io/my-org/modules/my-app:1.0.0 --key=cosign.pub
```

To verify the module signature while pulling:

```shell
timoni mod pull oci://ghcr.io/my-org/modules/my-app -v 1.0.0 \
  --output ./my-module \
  --verify=cosign \
  --cosign-key=cosign.pub
```

### Sign with Cosign keyless

For keyless signing, the Cosign CLI would prompt you to confirm that your email will be stored
in the public transparency logs. Timoni adds `--yes` to the cosign command to prevents this prompt.

Using Timoni with Cosign keyless signature means that users agree to this statement:

```text
Note that there may be personally identifiable information associated with this signed artifact.
This may include the email address associated with the account with which you authenticate.
This information will be used for signing this artifact and will be stored in public transparency logs and cannot be removed later.

By typing 'y', you attest that you grant (or have permission to grant) and agree to have this information stored permanently in transparency logs.
```

Sign the module while pushing:

```shell
timoni mod push ./modules/my-app oci://ghcr.io/my-org/modules/my-app \
  --version=1.0.0 \
  --sign=cosign
```

!!! tip "Signing in CI"

    When using `timoni push` in CI workflows, you can configure [GitHub](https://github.com/marketplace/actions/cosign-installer) and
    [GitLab](https://docs.gitlab.com/ee/ci/yaml/signing_examples.html) to provide Cosign with an OIDC token. 

    To automate the publishing and signing of module versions, please see the [Timoni GitHub Actions](github-actions.md).

To verify the module signature:

```shell
cosign verify ghcr.io/my-org/modules/my-app:1.0.0 \
  --certificate-identity-regexp=<your email address> \
  --certificate-oidc-issuer-regexp=<your issuer URL>
```

To verify the module signature while pulling:

```shell
timoni mod pull oci://ghcr.io/my-org/modules/my-app -v 1.0.0 \
  --output ./my-module \
  --verify=cosign \
  --certificate-identity-regexp=<your email address> \
  --certificate-oidc-issuer-regexp=<your issuer URL>
```
