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
				version: "7.0.12"
				digest:  "sha256:b58f3af3ef7a8e588a77a2a2f37af0a9ffc408fb27ff3f6ecbd2db42cf2a4297"
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
				version: "6.3.6"
				digest:  "sha256:b317bc7e4736287af10cbf52f9c0f1e8cd1f8c180bba29bc563fe697bfa2dd3d"
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
