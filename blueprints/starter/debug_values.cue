@if(debug)

package main

// Values used by debug_tool.cue.
// Debug example 'cue cmd -t debug -t name=test -t namespace=test -t mv=1.0.0 -t kv=1.28.0 build'.
values: {
	image: {
		repository: "docker.io/nginx"
		tag:        "1-alpine-slim"
		digest:     ""
	}

	pod: {
		annotations: "cluster-autoscaler.kubernetes.io/safe-to-evict": "true"
		imagePullSecrets: [{
			name: "regcred"
		}]
	}

	resources: {
		limits: {
			cpu:    "100m"
			memory: "128Mi"
		}
	}
}
