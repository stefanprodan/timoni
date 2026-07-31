// To deploy this bundle:
// timoni bundle apply -f podinfo.cue -f podinfo_secrets.cue

// This bundle defines a Redis master-replica cluster and
// a podinfo instance connected to Redis using the supplied password.
bundle: {
	apiVersion: "v1alpha1"
	name:       "podinfo"
	instances: {
		redis: {
			module: {
				url:     "oci://ghcr.io/stefanprodan/modules/redis"
				version: "8.10.0"
				digest:  "sha256:4f377b66fd6d608f6a347bbef5b4fcd1aaff3dfa253b1c8437ae8e11fa08b70a"
			}
			namespace: "podinfo"
			values: {
				maxmemory: 256
				readonly: replicas: 1
				password: _secrets.password
			}
		}
		podinfo: {
			module: {
				url:     "oci://ghcr.io/stefanprodan/modules/podinfo"
				version: "6.14.1"
				digest:  "sha256:ddf8be195f69ad767b40d5c3fec3a8a85482bf6bf469d949a462cb4fd27df580"
			}
			namespace: "podinfo"
			values: caching: {
				enabled:  true
				redisURL: "tcp://:\(_secrets.password)@redis:6379"
			}
		}
	}
}

// The secret values are defined in a separate file which
// can be kept encrypted or pulled from a secure vault at apply time.
_secrets: {
	password: string
}
