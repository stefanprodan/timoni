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

| KEY                          | TYPE                 | DEFAULT                                                                     | DESCRIPTION                     |
|------------------------------|----------------------|-----------------------------------------------------------------------------|---------------------------------|
| `metadata:`                  | `timoniv1.#Metadata` |                                                                             | Common metadata for all objects |
| `client: enabled:`           | `bool`               | `true`                                                                      |                                 |
| `client: image: repository:` | `string`             | `"cgr.dev/chainguard/timoni"`                                               |                                 |
| `client: image: tag:`        | `string`             | `"latest-dev"`                                                              |                                 |
| `client: image: digest:`     | `string`             | `"sha256:b49fbaac0eedc22c1cfcd26684707179cccbed0df205171bae3e1bae61326a10"` |                                 |
| `client: image: pullPolicy:` | `string`             | `"IfNotPresent"`                                                            |                                 |
| `server: enabled:`           | `bool`               | `true`                                                                      |                                 |
| `domain:`                    | `string`             | `"example.internal"`                                                        |                                 |
| `podAnnotations?:`           | `{[string]: string}` |                                                                             | Annotations applied to pods     |
| `logLevel?:`                 | `"debug" \| "info"`  | `"info"`                                                                    | Log level, info by default      |
| `priority:`                  | `int & >=0`          | `1`                                                                         |                                 |
| `globals: enabled:`          | `bool`               | `false`                                                                     |                                 |
| `team:`                      | `"test"`             | `"test"`                                                                    |                                 |

