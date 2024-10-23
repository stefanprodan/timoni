# module

A [timoni.sh](http://timoni.sh) module for deploying blueprint to Kubernetes clusters.

## Install

To create an instance using the default values:

```shell
timoni -n module apply module oci://<container-registry-url>
```

To change the [default configuration](#configuration),
create one or more `values.cue` files and apply them to the instance.

For example, create a file `my-values.cue` with the following content:

```cue
values: {
    team: "timoni"
    metadata: labels: testing: "true"
    domain: "example.com"
    ns: enabled: true
}
```

And apply the values with:

```shell
timoni -n module apply module oci://<container-registry-url> \
--values ./my-values.cue
```

## Uninstall

To uninstall an instance and delete all its Kubernetes resources:

```shell
timoni -n module delete module
```

## Configuration

| KEY                          | TYPE                                                                                   | DESCRIPTION                                                                                                                                                                                                                                   |
|------------------------------|----------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `client: enabled:`           | `*true \| bool`                                                                        |                                                                                                                                                                                                                                               |
| `client: image: repository:` | `*"cgr.dev/chainguard/timoni" \| string`                                               | Repository is the address of a container registry repository. An image repository is made up of slash-separated name components, optionally prefixed by a registry hostname and port in the format [HOST[:PORT_NUMBER]/]PATH.                 |
| `client: image: tag:`        | `*"latest-dev" \| strings.MaxRunes(128)`                                               | Tag identifies an image in the repository. A tag name may contain lowercase and uppercase characters, digits, underscores, periods and dashes. A tag name may not start with a period or a dash and may contain a maximum of 128 characters.  |
| `client: image: digest:`     | `*"sha256:b49fbaac0eedc22c1cfcd26684707179cccbed0df205171bae3e1bae61326a10" \| string` | Digest uniquely and immutably identifies an image in the repository. Spec: https://github.com/opencontainers/image-spec/blob/main/descriptor.md#digests.                                                                                      |
| `server: enabled:`           | `*true \| bool`                                                                        |                                                                                                                                                                                                                                               |
| `domain:`                    | `*"example.internal" \| string`                                                        |                                                                                                                                                                                                                                               |
| `ns: enabled:`               | `*false \| bool`                                                                       |                                                                                                                                                                                                                                               |
| `team:`                      | `"test"`                                                                               |                                                                                                                                                                                                                                               |

