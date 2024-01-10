# Module Signing and Verification

Timoni modules are distributed as OCI artifacts. When publishing a module version to a container registry,
the OCI artifact can be cryptographically signed to improve the software supply chain security.

## Cosign

Timoni can sign modules using Sigstore Cosign.
[Cosign](https://github.com/sigstore/cosign) is a tool that allows you to sign and verify
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

!!! tip "Verify Signature from github action"

    When the signature was created in github via the keyless signing you should set the flag `--certificate-identity-regexp`
    to a value like `^https://github.com/<user/org>/<repo-name>.*`.

Example verification of the podinfo module:

```shell
cosign verify ghcr.io/stefanprodan/modules/podinfo:latest --certificate-identity-regexp="^https://github.com/stefanprodan/podinfo.*$" --certificate-oidc-issuer=https://token.actions.githubusercontent.com

Verification for ghcr.io/stefanprodan/modules/podinfo:latest --
The following checks were performed on each of these signatures:
  - The cosign claims were validated
  - Existence of the claims in the transparency log was verified offline
  - The code-signing certificate was verified using trusted certificate authority certificates

[{"critical":{"identity":{"docker-reference":"ghcr.io/stefanprodan/modules/podinfo"},"image":{"docker-manifest-digest":"sha256:1dba385f9d56f9a79e5b87344bbec1502bd11f056df51834e18d3e054de39365"},"type":"cosign container image signature"},"optional":{"1.3.6.1.4.1.57264.1.1":"https://token.actions.githubusercontent.com","1.3.6.1.4.1.57264.1.2":"push","1.3.6.1.4.1.57264.1.3":"33dac1ba40f73555725fbf620bf3b4f6f1a5ad89","1.3.6.1.4.1.57264.1.4":"release","1.3.6.1.4.1.57264.1.5":"stefanprodan/podinfo","1.3.6.1.4.1.57264.1.6":"refs/tags/6.5.4","Bundle":{"SignedEntryTimestamp":"MEUCIQDWtqRAS+R/nyPfz5a+RLonpTwVJNYzFDPBIDHBW1ZN5AIgUvr8U5qB6AYUfnZwNjhFSwNCP+7XrJB48NTIYVp1U+Y=","Payload":{"body":"eyJhcGlWZXJzaW9uIjoiMC4wLjEiLCJraW5kIjoiaGFzaGVkcmVrb3JkIiwic3BlYyI6eyJkYXRhIjp7Imhhc2giOnsiYWxnb3JpdGhtIjoic2hhMjU2IiwidmFsdWUiOiJiNTBmMjY1YTEzZGEyNjEyOTVjZjE0ODU2MzQzOTJkODgxZDNjMzlhNWRkMTk2NzFlOWJhM2YwZDBlMjU3ZjE5In19LCJzaWduYXR1cmUiOnsiY29udGVudCI6Ik1FUUNJQ1ptbHNHRmhnN0ltT3dlY2h6aW9ncHkzV1BOY0hDdFphcjFBWnR3blhHekFpQVJ6Z0k5SWNaL3dzOE5FdTdzWVVRVHNLcDhVYVdFQVpQTWM5L3BZZzU2Q3c9PSIsInB1YmxpY0tleSI6eyJjb250ZW50IjoiTFMwdExTMUNSVWRKVGlCRFJWSlVTVVpKUTBGVVJTMHRMUzB0Q2sxSlNVZDFSRU5EUW1veVowRjNTVUpCWjBsVlltSmpRV052UTFSbVFYSjNSakpIUmxwVGFWRTJWa3QyTTNCRmQwTm5XVWxMYjFwSmVtb3dSVUYzVFhjS1RucEZWazFDVFVkQk1WVkZRMmhOVFdNeWJHNWpNMUoyWTIxVmRWcEhWakpOVWpSM1NFRlpSRlpSVVVSRmVGWjZZVmRrZW1SSE9YbGFVekZ3WW01U2JBcGpiVEZzV2tkc2FHUkhWWGRJYUdOT1RXcE5lRTFxUlROTlZGRXdUbnBGTUZkb1kwNU5hazE0VFdwRk0wMVVVVEZPZWtVd1YycEJRVTFHYTNkRmQxbElDa3R2V2tsNmFqQkRRVkZaU1V0dldrbDZhakJFUVZGalJGRm5RVVVyTDI5VFIwNVZOM1l4TDFCUWRETm1Ta1ExYmpZMmIxb3JPU3RGZEdJeFpUVXlObUlLYmtOVGVrWnlVMWhQUWtWaWJEWm1MeXN4Y0RRclNHUkxNakl4SzI1emRVcExiVFJqUm1WcmRYaFpRbVl2UWpaNWJrdFBRMEpXZDNkbloxWlpUVUUwUndwQk1WVmtSSGRGUWk5M1VVVkJkMGxJWjBSQlZFSm5UbFpJVTFWRlJFUkJTMEpuWjNKQ1owVkdRbEZqUkVGNlFXUkNaMDVXU0ZFMFJVWm5VVlY0UVRabkNubHJkbWRyZFc0M1dIWXZTV1p6U1VSWFpXUjJOMmMwZDBoM1dVUldVakJxUWtKbmQwWnZRVlV6T1ZCd2VqRlphMFZhWWpWeFRtcHdTMFpYYVhocE5Ga0tXa1E0ZDFsM1dVUldVakJTUVZGSUwwSkdhM2RXTkZwV1lVaFNNR05JVFRaTWVUbHVZVmhTYjJSWFNYVlpNamwwVEROT01GcFhXbWhpYmtKNVlqSlNhQXBpYVRsM1lqSlNjR0p0V25aTWVUVnVZVmhTYjJSWFNYWmtNamw1WVRKYWMySXpaSHBNTTBwc1lrZFdhR015VlhWbFZ6RnpVVWhLYkZwdVRYWmtSMFp1Q21ONU9ESk1hbFYxVGtSQk5VSm5iM0pDWjBWRlFWbFBMMDFCUlVKQ1EzUnZaRWhTZDJONmIzWk1NMUoyWVRKV2RVeHRSbXBrUjJ4MlltNU5kVm95YkRBS1lVaFdhV1JZVG14amJVNTJZbTVTYkdKdVVYVlpNamwwVFVKSlIwTnBjMGRCVVZGQ1p6YzRkMEZSU1VWQ1NFSXhZekpuZDA1bldVdExkMWxDUWtGSFJBcDJla0ZDUVhkUmIwMTZUbXRaVjAxNFdXMUZNRTFIV1ROTmVsVXhUbFJqZVU1WFdtbGFhbGw1VFVkS2JVMHlTVEJhYWxwdFRWZEZNVmxYVVRSUFZFRldDa0puYjNKQ1owVkZRVmxQTDAxQlJVVkNRV1I1V2xkNGJGbFlUbXhOUTBsSFEybHpSMEZSVVVKbk56aDNRVkZWUlVaSVRqQmFWMXBvWW01Q2VXSXlVbWdLWW1rNWQySXlVbkJpYlZwMlRVSXdSME5wYzBkQlVWRkNaemM0ZDBGUldVVkVNMHBzV201TmRtUkhSbTVqZVRneVRHcFZkVTVFUVRkQ1oyOXlRbWRGUlFwQldVOHZUVUZGU1VKRE1FMUxNbWd3WkVoQ2VrOXBPSFprUnpseVdsYzBkVmxYVGpCaFZ6bDFZM2sxYm1GWVVtOWtWMG94WXpKV2VWa3lPWFZrUjFaMUNtUkROV3BpTWpCM1dsRlpTMHQzV1VKQ1FVZEVkbnBCUWtOUlVsaEVSbFp2WkVoU2QyTjZiM1pNTW1Sd1pFZG9NVmxwTldwaU1qQjJZek5TYkZwdFJuVUtZMGhLZGxwSFJuVk1NMEoyV2tkc2RWcHRPSFpNYldSd1pFZG9NVmxwT1ROaU0wcHlXbTE0ZG1RelRYWmpiVlp6V2xkR2VscFROVFZpVjNoQlkyMVdiUXBqZVRrd1dWZGtla3g2V1hWT1V6UXdUVVJuUjBOcGMwZEJVVkZDWnpjNGQwRlJiMFZMWjNkdlRYcE9hMWxYVFhoWmJVVXdUVWRaTTAxNlZURk9WR041Q2s1WFdtbGFhbGw1VFVkS2JVMHlTVEJhYWxwdFRWZEZNVmxYVVRSUFZFRmtRbWR2Y2tKblJVVkJXVTh2VFVGRlRFSkJPRTFFVjJSd1pFZG9NVmxwTVc4S1lqTk9NRnBYVVhkT2QxbExTM2RaUWtKQlIwUjJla0ZDUkVGUmNFUkRaRzlrU0ZKM1kzcHZka3d5WkhCa1IyZ3hXV2sxYW1JeU1IWmpNMUpzV20xR2RRcGpTRXAyV2tkR2RVd3pRblphUjJ4MVdtMDRkMDlCV1V0TGQxbENRa0ZIUkhaNlFVSkVVVkZ4UkVObmVrMHlVbWhaZWtacFdWUlJkMXBxWTNwT1ZGVXhDazU2U1RGYWJVcHRUbXBKZDFsdFdYcFphbEp0VG0xWmVGbFVWbWhhUkdjMVRVSTRSME5wYzBkQlVWRkNaemM0ZDBGUk5FVkZVWGRRWTIxV2JXTjVPVEFLV1Zka2VreDZXWFZPVXpRd1RVSnJSME5wYzBkQlVWRkNaemM0ZDBGUk9FVkRkM2RLVFZSRk1rMTZhM2xPVkd0M1RVTTRSME5wYzBkQlVWRkNaemM0ZHdwQlVrRkZTVkYzWm1GSVVqQmpTRTAyVEhrNWJtRllVbTlrVjBsMVdUSTVkRXd6VGpCYVYxcG9ZbTVDZVdJeVVtaGlha0ZZUW1kdmNrSm5SVVZCV1U4dkNrMUJSVkpDUVd0TlFucE5NMDlVWXpKT2VsVjNXbEZaUzB0M1dVSkNRVWRFZG5wQlFrVm5VbGhFUmxadlpFaFNkMk42YjNaTU1tUndaRWRvTVZscE5Xb0tZakl3ZG1NelVteGFiVVoxWTBoS2RscEhSblZNTTBKMldrZHNkVnB0T0haTWJXUndaRWRvTVZscE9UTmlNMHB5V20xNGRtUXpUWFpqYlZaeldsZEdlZ3BhVXpVMVlsZDRRV050Vm0xamVUa3dXVmRrZWt4NldYVk9VelF3VFVSblIwTnBjMGRCVVZGQ1p6YzRkMEZTVFVWTFozZHZUWHBPYTFsWFRYaFpiVVV3Q2sxSFdUTk5lbFV4VGxSamVVNVhXbWxhYWxsNVRVZEtiVTB5U1RCYWFscHRUVmRGTVZsWFVUUlBWRUZWUW1kdmNrSm5SVVZCV1U4dlRVRkZWVUpCV1UwS1FraENNV015WjNkWFoxbExTM2RaUWtKQlIwUjJla0ZDUmxGU1RVUkZjRzlrU0ZKM1kzcHZka3d5WkhCa1IyZ3hXV2sxYW1JeU1IWmpNMUpzV20xR2RRcGpTRXAyV2tkR2RVd3pRblphUjJ4MVdtMDRkbGxYVGpCaFZ6bDFZM2s1ZVdSWE5YcE1lbU41VFhwcmVFOVVWVFJPUkd0MldWaFNNRnBYTVhka1NFMTJDazFVUVZkQ1oyOXlRbWRGUlVGWlR5OU5RVVZYUWtGblRVSnVRakZaYlhod1dYcERRbWxSV1V0TGQxbENRa0ZJVjJWUlNVVkJaMUkzUWtoclFXUjNRakVLUVU0d09VMUhja2Q0ZUVWNVdYaHJaVWhLYkc1T2QwdHBVMncyTkROcWVYUXZOR1ZMWTI5QmRrdGxOazlCUVVGQ2FraG5LekprV1VGQlFWRkVRVVZaZHdwU1FVbG5UeTlPY1U5dVRFZHVibnBUT1ZOM2ExQjBLek5UZEhGWlozaHBTMjE0U21Vd1VtNUliSFJGYUZVeFFVTkpRVzR3YmtOT01HNXdUazFGWjJvMUNsQnhWM01yTld4cWVsbEJkblZ6UVdkcVoySjVXVWRGTVVKbGNtRk5RVzlIUTBOeFIxTk5ORGxDUVUxRVFUSnJRVTFIV1VOTlVVUjJNR29yVjJJdk16WUtTamRwWkdGT2FXRjVlVTl3V1Rkb00zZHhRMEp4YjBGS1V6UnhMeTlVYm5sRk5XTk9NMG81ZEd0RVNqbFlkRzQwYW5sdWFFeEZWVU5OVVVSelVrZFFRZ3BGYlZvd2RHMUxjQ3R3VVhkc1N6Vm9ZbWRWT1cxUWJtcFpSVmR2YTFsME9HY3hia3d5VVc1WlUxVXhkVTl1TjFwUWFUSkhNM0pzVjFSRGN6MEtMUzB0TFMxRlRrUWdRMFZTVkVsR1NVTkJWRVV0TFMwdExRbz0ifX19fQ==","integratedTime":1702824434,"logIndex":57409478,"logID":"c0d23d6ad406973f9559f3ba2d1ca01f84147d8ffc5b8445c224f98b9591801d"}},"Issuer":"https://token.actions.githubusercontent.com","Subject":"https://github.com/stefanprodan/podinfo/.github/workflows/release.yml@refs/tags/6.5.4","githubWorkflowName":"release","githubWorkflowRef":"refs/tags/6.5.4","githubWorkflowRepository":"stefanprodan/podinfo","githubWorkflowSha":"33dac1ba40f73555725fbf620bf3b4f6f1a5ad89","githubWorkflowTrigger":"push"}}]
```

To verify the module signature while pulling:

```shell
timoni mod pull oci://ghcr.io/my-org/modules/my-app -v 1.0.0 \
  --output ./my-module \
  --verify=cosign \
  --certificate-identity-regexp=<your email address> \
  --certificate-oidc-issuer-regexp=<your issuer URL>
```
