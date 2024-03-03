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
				version: "7.2.4"
				digest:  "sha256:8cf531365742c7cab9628909dfe16958550853f7c994284eacad64f169f4c74a"
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
				version: "6.5.3"
				digest:  "sha256:54d38b407012ccfb42badf0974ba70f9ae229ecd38f17e8a1f4e7189283b924f"
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
