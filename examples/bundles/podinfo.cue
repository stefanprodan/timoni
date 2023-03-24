// To deploy this bundle:
// timoni bundle apply -f podinfo.cue -f podinfo_secrets.cue

// This bundle defines a Redis master-replica cluster and
// a podinfo instance connected to Redis using the supplied password.
bundle: {
	apiVersion: "v1alpha1"
	instances: {
		redis: {
			module: {
				url:     "oci://ghcr.io/stefanprodan/modules/redis"
				version: "7.0.9"
			}
			namespace: "podinfo"
			values: {
				maxmemory: 256
				readonly: replicas: 1
				password: _secrets.password
			}
		}
		podinfo: {
			module: url:     "oci://ghcr.io/stefanprodan/modules/podinfo"
			module: version: "6.3.4"
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
