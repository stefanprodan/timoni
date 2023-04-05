# Values

Values are used to customise a module instance by providing Timoni with a config object.
The config schema, constraints and the default values are contained in the module definition.

## Values files

Values can be supplied as CUE, YAML or JSON files
to the `timoni apply <instance-name> [--values <path/to/values>]` command.

Given the config definition:

```go
#Config: {
	image: {
		repository: *"docker.io/stefanprodan/podinfo" | string
		tag:        *"6.3.0" | string
		pullPolicy: *"IfNotPresent" | string
	}
	replicas:   *1 | int & >0
	resources?: corev1.#ResourceRequirements
}
```

A values file can override default values such as an image repository:

```go
values: {
	image: repository: "ghcr.io/stefanprodan/podinfo"
}
```

And set optional values such as resources limits:

```go
values: {
	resources: limits: {
		cpu:    "100m"
		memory: "128Mi"
	}
}
```

To create an instance using the custom values, both files can be supplied with:

```shell
timoni -n default apply podinfo \
  oci://ghcr.io/stefanprodan/modules/podinfo \
  --values image-values.cue \
  --values limits-values.cue
```

Values can also be supplied by piping a CUE object, for example:

```shell
echo "values: replicas: 2" | timoni -n default apply podinfo \
  oci://ghcr.io/stefanprodan/modules/podinfo \
  --values image-values.cue \
  --values limits-values.cue
```

At apply time, Timoni merges the values, validates them
against the config schema and creates the instance.

When values are supplied as CUE files, they can contain arithmetic operations,
string interpolation and everything else that CUE std lib supports.

For example, to set the resources limits to 2x requests:

```go
values: {
	_mcpu: 500
	_mem:  256
	resources: {
		requests: {
			cpu:    "\(_mcpu)m"
			memory: "\(_mem)Mi"
		}
		limits: {
			cpu:    "\(_mcpu*2)m"
			memory: "\(_mem*2)Mi"
		}
	}
}
```
