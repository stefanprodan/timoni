# Bundle Runtime

While Timoni [Bundles](bundle.md) offer a way to specify the config values in declarative manner,
not all the configuration values of an application are known ahead of time.
Some values may be available at runtime, in the Kubernetes clusters where the Bundle is applied.

For example, the API token for some backend service that your app consumes is stored in
a Kubernetes Secret in-cluster. When installing the application with Timoni,
you may want to fetch the token from the Kubernetes Secret and use it as a config value.

Timoni Runtime API offers a way to define values which are fetched from the Kubernetes API
and mapped to fields inside a Bundle.

## Example

The following is an example of a Runtime definition that extracts values from the cluster.

```cue
runtime: {
	apiVersion: "v1alpha1"
	name:       "production"
	clusters: {
		// using the cluster set in kubeconfig current context
	}
	values: [
		{
			query: "k8s:v1:ConfigMap:infra:aws-info"
			for: {
				"ACCOUNT_ID": "obj.data.account_id"
				"REGION":     "obj.data.region"
			}
		},
		{
			query: "k8s:v1:Secret:infra:redis-auth"
			for: {
				"REDIS_PASS": "obj.data.password"
				"REDIS_CA":   "obj.data.\"ca.crt\""
			}
		},
	]
}
```

The values from the Runtime, can be mapped to fields in a Bundle using `@timoni()` attributes.

```cue
bundle: {
	_id:   string @timoni(runtime:string:ACCOUNT_ID)
	_reg:  string @timoni(runtime:string:REGION)
	_pass: string @timoni(runtime:string:REDIS_PASS)

	apiVersion: "v1alpha1"
	name:       "podinfo"
	instances: {
		podinfo: {
			module: url: "oci://\(_id).dkr.ecr.\(_reg).amazonaws.com/modules/podinfo"
			namespace: "podinfo"
			values: caching: {
				enabled:  true
				redisURL: "tcp://:\(_pass)@redis.infra:6379"
			}
		}
	}
}
```

Assuming the ConfigMaps and Secrets are in the cluster,
and the Runtime file is `runtime.cue` and the Bundle file is `bundle.cue`.

Build the runtime to see which values are present on the cluster:

=== "command"

      ```sh
      timoni runtime build -f runtime.cue
      ```

=== "output"

      ```text
      ACCOUNT_ID: 1234567890
      REGION: us-west-2
      REDIS_PASS: password
      REDIS_CA:
        -----BEGIN CERTIFICATE-----
        MIIC/jCCAeagAwIBAgIBADANBgkqhkiG9w0BAQsFADAVMRMwEQYDVQQDEwprdWJl
        cm5ldGVzMB4XDTIzMDgxMDE1MTA1MFoXDTMzMDgwNzE1MTA1MFowFTETMBEGA1UE
        AxMKa3ViZXJuZXRlczCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBAJgA
        mOniTayGyXG51eYeb1oJGCszRlZv4ci2kzLC0H4YCph6TXDEawrsvyuzGe7fwp4t
        Jq2fQiJgKbibOjHcOwhYqp1lJDu37p6WsLvx/RxbkHoVX8X2n7n1BVS5MbQ81on9
        BdXkff71g8n3HmJo8ZN1abEW434tHFwjjGMDzFoqxTnNM+qMKegxAtTPVbjV4X+5
        4/95L7T0XNd97XnunSUAEJRroMTRuaBAcYz81N0ix3Mc0T+G16aV127R0ZFkPlS8
        bzjA1NGgbiveX7i7n9mNG4Gy7iElP0iidtdrA6loxWYLW1jmEH5/pWQnki7s3OrK
        /94xrQ2MaDpXUzcoc0cCAwEAAaNZMFcwDgYDVR0PAQH/BAQDAgKkMA8GA1UdEwEB
        /wQFMAMBAf8wHQYDVR0OBBYEFJ7f9QzKWA/BQJSRinKne4FZNzScMBUGA1UdEQQO
        MAyCCmt1YmVybmV0ZXMwDQYJKoZIhvcNAQELBQADggEBAAhGgYFB90Rkexgh56q8
        n8uC4XQF6OTJJf/4A9VXNbiZhG+PUYkMUyqeOSOq+IztJVOffuG9ZutYQtJYCX7g
        TUQmZxeKAw1MAMEWAELmNVdbG6Y6YZ14v8SI+TADqeqcIkmDEkACygfeHKdEFOfV
        LoRAc3Zn32byP5M6EVjTXWvb+UbVt7umoBOCbMo3BEF71TEDI5Oww6y3YxDM9/Y7
        MCTu+0XHZNGDgprmci6fzLv8PV0C6eiuUrrBM237WOfk4BYL6pmCDcCWo6aP2cH7
        Y5zcVBxKUs/h5bZfLJFuwXJC5zWudNLOAtNtFhctMwDoNaKYq720g/GmEroq4wgA
        tBI=
        -----END CERTIFICATE-----
      ```

Apply the Bundle using the values from the Runtime:

=== "command"

      ```sh
      timoni bundle apply -f bundle.cue --runtime runtime.cue
      ```

=== "output"

      ```text
      applying instance podinfo
      pulling oci://1234567890.dkr.ecr.us-west-2.amazonaws.com/modules/podinfo
      using module timoni.sh/podinfo version 6.3.5
      installing podinfo in namespace podinfo
      ServiceAccount/podinfo/podinfo created
      Service/podinfo/podinfo created
      Deployment/podinfo/podinfo created
      waiting for 3 resource(s) to become ready...
      resources are ready
      ```

## Writing a Runtime spec

A Runtime file must contain a definition that matches the following schema:

```cue
#Runtime: {
	apiVersion: string
	name:       string
	
	clusters?: [string]: {
		group!:       string
		kubeContext!: string
	}
	
	values?: [...#RuntimeValue]
}

#RuntimeValue: {
	query: string
	for: {[string]: string}
	optional: *false | bool
}
```

### API version

The `apiVersion` is a required field that specifies the version of the Runtime schema.

Currently, the only supported value is `v1alpha1`.

### Name

The `name` is a required field used to identify the Runtime.

### Clusters

The `clusters` field is for defining the target clusters and
environments (group of clusters) where a Bundle is applied.

A cluster entry must specify the `group` and `kubeContext` fields.
The `kubeContext` value must match a context name from the `.kube/config` file.

!!! tip "Default cluster"
    
    When no clusters are defined in the Runtime, Timoni will use the
    current context from the kubeconfig, unless the context is specifed
    using the `--kube-context` flag.

Example:

```cue
runtime: {
	apiVersion: "v1alpha1"
	name:       "fleet"
	clusters: {
		"preview-us-1": {
			group:       "staging"
			kubeContext: "eks-us-west-2"
		}
		"prod-us-1": {
			group:       "production"
			kubeContext: "eks-us-west-1"
		}
		"prod-eu-1": {
			group:       "production"
			kubeContext: "eks-eu-west-1"
		}
	}
}
```

The clusters name and group, can be mapped to fields in a Bundle using `@timoni()` attributes.

```cue
bundle: {
	_cluster: string @timoni(runtime:string:TIMONI_CLUSTER_NAME)
	_env:     string @timoni(runtime:string:TIMONI_CLUSTER_GROUP)

	apiVersion: "v1alpha1"
	name:       "apps"
	instances: {
		app: {
			module: url: "oci://ghcr.io/stefanprodan/modules/podinfo"
			namespace: "apps"
			values: {
				ui: message: "Hosted by \(_cluster)"
				if _env == "staging" {
					replicas: 1
				}
				if _env == "production" {
					replicas: 2
				}
			}
		}
	}
}
```

When applying the above Bundle, Timoni will deploy the app instances to all the
clusters, in the order defined in the Runtime. If the apply fails on a staging cluster,
Timoni will stop the execution and not continue with production.

For more details please see the [multi-cluster deployments guide](bundle-multi-cluster.md).

### Values

The `values` array is for specifying
the list of Kubernetes resources and the fields to be extracted.

#### Query

The `values.query` is a required field that specifies the Kubernetes resource.

The `query` field must be in the format `k8s:<apiVersion>:<kind>:<namespace>:<name>`.

Example:

```cue
runtime: {
	apiVersion: "v1alpha1"
	name:       "production"
	values: [
		{
			query: "k8s:v1:Secret:infra:redis-auth"
			for: {
				"REDIS_PASS": "obj.data.password"
			}
		},
	]
}
```

If the Kubernetes resource is global, the `query` format is `k8s:<apiVersion>:<kind>:<name>`.

Example:

```cue
runtime: {
	apiVersion: "v1alpha1"
	name:       "production"
	values: [
		{
			query: "k8s:cert-manager.io/v1:ClusterIssuer:letsencrypt"
			for: {
				"ISSUER_EMAIL": "obj.spec.acme.email"
			}
		},
	]
}
```

#### For

The `values.for` is a required map that specifies which fields to be extracted from the Kubernetes resource.

The `for` map must contain pairs of name and CUE expression.

Example:

```cue
runtime: {
	apiVersion: "v1alpha1"
	name:       "production"
	values: [
		{
			query: "k8s:source.toolkit.fluxcd.io/v1:GitRepository:flux-system:cluster"
			for: {
				"GIT_REVISION": "obj.status.artifact.revision"
				"GIT_STATUS":   "[for c in obj.status.conditions if c.type == \"Ready\" {c.status}][0]"
			}
		},
	]
}
```

The CUE expression must result in a concrete value of type `string`, `number` or `bool`.

#### Optional

The `optional` field can be set to `true` and Timoni will skip
not found Kubernetes resources instead of throwing an error.

## Using values from Kubernetes API

The values defined in a Runtime can be referred in Bundles using CUE attributes.

The `@timoni(runtime:[string|number|bool]:[VAR_NAME])` CUE attribute can be placed next
to a field to set its value from the runtime.

```cue
bundle: {
	apiVersion: "v1alpha1"
	name:       "app"
	instances: {
		app: {
			module: url: "oci://localhost:5000/modules/app"
			namespace: "apps"
			values: {
				host:    string @timoni(runtime:string:HOST)
				enabled: bool   @timoni(runtime:bool:ENABLED)
				score:   int    @timoni(runtime:number:SCORE)
			}
		}
	}
}
```

If a runtime value is optional, you can provide defaults which will be used
only if the runtime doesn't contain the referenced value:

```cue
values: {
	host:    "example.com" @timoni(runtime:string:HOST)
	enabled: true          @timoni(runtime:bool:ENABLED)
	score:   1             @timoni(runtime:number:SCORE)
}
```

To load the runtime values at apply time, use the `--runtime` flag:

```shell
timoni bundle apply -f bundle.cue --runtime runtime.cue
```

At apply time, Timoni injects the fields values from the runtime,
if a specified runtime var is not found and if a default is not provided,
the apply with fail with an `incomplete value` error.

## Using values from environment variables

To use values from environment variables,
the environment must contain variables which match the CUE attributes.

Example:

```cue
bundle: {
	apiVersion: "v1alpha1"
	name:       "app"
	instances: {
		app: {
			module: url: "oci://localhost:5000/modules/app"
			namespace: "apps"
			values: {
				sshKey:  string @timoni(runtime:string:SSH_KEY)
				isAdmin: false  @timoni(runtime:bool:IS_ADMIN)
			}
		}
	}
}
```

Export the env vars and run the `timoni bundle apply --runtime-from-env` command.

```shell
EXPORT SSH_KEY=$(cat .ssh/id_ecdsa.pub)
EXPORT IS_ADMIN="true"

timoni bundle apply -f bundle.cue --runtime-from-env
```

!!! tip "Mixing values from the Runtime and Environment"

    When using `timoni bundle apply --runtime runtime.cue --runtime-from-env`,
    the values coming from the Runtime take precedence over the Environment.
