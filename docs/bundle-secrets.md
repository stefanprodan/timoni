# Bundle Secrets Injection

Ways of injecting secrets when deploying applications with Timoni [Bundles](bundle.md):

- Using [runtime](#runtime-secrets) attributes `@timoni(runtime:string:SECRET-NAME)`.
- Using [SOPS](#sops-secrets) encrypted YAML or JSON files.

## Runtime Secrets

To showcase how secrets injections works, we'll assume we are deploying an application
that connects to an S3-compatible API and needs two secrets: `ACCESS_KEY` and `SECRET_KEY`.

### Injecting secrets from CI secret store

When using a CI runner to deploy apps with Timoni,
we can pass secrets from the runner's secret store to Timoni's Bundle.

Example of a Bundle that contains runtime attributes:

```cue
bundle: {
	apiVersion: "v1alpha1"
	name:       "my-app"
	instances: {
		"my-app-storage": {
			module: url: "oci://my-registry/timoni/modules/my-app-storage"
			namespace: "my-app"
			values: {
				endpoint:  "https://my-acc.r2.cloudflarestorage.com"
				accessKey: string @timoni(runtime:string:ACCESS_KEY)
				secretKey: string @timoni(runtime:string:SECRET_KEY)
			}
		}
	}
}
```

In a GitHub workflow, we can map secrets from GitHub secrets to env vars,
that Timoni will use at apply-time:

```shell
export ACCESS_KEY=${{ secrets.ACCESS_KEY }}
export SECRET_KEY=${{ secrets.SECRET_KEY }}

timoni bundle apply -f bundle.cue --runtime-from-env
```

### Injecting secrets from Kubernetes

The same secrets from the above example, can be injected from a Kubernetes Secret,
assuming we're using some external-secret controller that syncs secrets from a Vault in etcd.

Example of a Timoni [Bundle Runtime](bundle-runtime.md) that fetches the secrets from the cluster:

```cue
runtime: {
	apiVersion: "v1alpha1"
	name:       "production"
	values: [
		{
			query: "k8s:v1:Secret:my-namespace:my-secret-name"
			for: {
				"ACCESS_KEY": "obj.data.r2_access_key"
				"SECRET_KEY": "obj.data.r2_secret_key"
			}
		},
	]
}
```

At apply-time we pass the runtime definition and
Timoni will read the secrets from the Kubernetes cluster
and use them when applying the bundle:

```shell
timoni bundle apply -f bundle.cue --runtime runtime.cue
```

### Secrets interpolation

When the secrets stored in external system do not map to a value key in the Bundle,
we can use CUE's string interpolation to compose the desired value.

Assuming an application config expects a Redis URL, but the secret store contains
`REDIS_HOST` and `REDIS_PASS`.

```cue
bundle: {
	apiVersion: "v1alpha1"
	name:       "podinfo"
	_secrets: {
		host:     string @timoni(runtime:string:REDIS_HOST)
		password: string @timoni(runtime:string:REDIS_PASS)
	}
	instances: {
		"podinfo-backend": {
			module: url: "oci://ghcr.io/stefanprodan/modules/podinfo"
			namespace: "podinfo"
			values: caching: {
				enabled:  true
				redisURL: "tcp://:\(_secrets.password)@\(_secrets.host):6379"
			}
		}
	}
}
```

In the above example, we define a CUE hidden filed `_secrets`, where we set the
runtime secrets mappings. Then in the instance values, we use string interpolation
to set the `redisURL` containing the secrets.

Using the build command, we can see the URL value set in the podinfo container args:

```console
$ export REDIS_HOST=redis.svc
$ export REDIS_PASS=testpass
$ timoni bundle build -f bundle.cue --runtime-from-env | grep redis
        - --cache-server=tcp://:testpass@redis.svc:6379
```

## SOPS secrets

When using [SOPS](https://github.com/getsops/sops),
we can decrypt the secrets and [inject](https://github.com/getsops/sops#passing-secrets-to-other-processes)
those values to env vars, then use `--runtime-from-env`.

Another option is to extract the secret values of a Timoni Bundle to an YAML or JSON file,
that we encrypt/decrypt with SOPS.

### Injecting secrets from SOPS

Main bundle file `bundle.main.cue`:

```cue
bundle: {
	apiVersion: "v1alpha1"
	name:       "my-app"
	instances: {
		"my-app-storage": {
			module: url: "oci://my-registry/timoni/modules/my-app-storage"
			namespace: "my-app"
			values: {
				endpoint:  "https://my-acc.r2.cloudflarestorage.com"
				// The secrets are omitted here!
			}
		}
	}
}
```

Bundle partial in YAML format `bundle.secret.yaml`:

```yaml
bundle:
  instances:
    my-app-storage:
      values:
        accessKey: ENC[AES256_GCM,data:..]
        secretKey: ENC[AES256_GCM,data:..]
```

Assuming the `bundle.secret.yaml` file is kept encrypted with SOPS,
at apply-time we can run the SOPS decryption,
and pass the plain YAML to Timoni's apply command like so:

```shell
sops exec-file --filename secrets.yml bundle.secret.yaml 'timoni bundle apply -f bundle.main.cue -f {}'
```
