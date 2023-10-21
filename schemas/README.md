# Timoni CUE Schemas

The Timoni's CUE schemas are included in the modules generated with `timoni mod init`.

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
