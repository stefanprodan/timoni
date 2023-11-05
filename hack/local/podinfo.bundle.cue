bundle: {
	apiVersion: "v1alpha1"
	name:       "podinfo"
	instances: {
		cache: {
			module: {
				url:     "oci://localhost:5555/modules/redis"
				version: "latest"
			}
			namespace: "podinfo"
			values: {
				maxmemory: 256
				readonly: replicas: 1
			}
		}
		backend: {
			module: {
				url:     "oci://localhost:5555/modules/podinfo"
				version: "latest"
			}
			namespace: "podinfo"
			values: caching: {
				enabled:  true
				redisURL: "tcp://cache:6379"
			}
			values: test: enabled: true
		}
		frontend: {
			module: {
				url:     "oci://localhost:5555/modules/podinfo"
				version: "latest"
			}
			namespace: "podinfo"
			values: {
				resources: {
					requests: cpu:  "100m"
					limits: memory: "128Mi"
				}
				podSecurityContext: {
					runAsUser:  100
					runAsGroup: 101
					fsGroup:    101
				}
				securityContext: {
					allowPrivilegeEscalation: false
					readOnlyRootFilesystem:   true
					runAsNonRoot:             true
					capabilities: drop: ["ALL"]
					seccompProfile: type: "RuntimeDefault"
				}
				test: enabled: true
			}
		}
	}
}
