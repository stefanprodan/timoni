@if(debug)

package main

// Values used by debug_tool.cue.
// Debug example 'cue cmd -t debug -t name=redis -t namespace=test -t mv=1.0.0 -t kv=1.28.0 build'.
values: {
	podAnnotations: "cluster-autoscaler.kubernetes.io/safe-to-evict": "true"
	image: {
		repository: "docker.io/redis"
		tag:        "7-alpine"
		digest:     ""
	}
	test: enabled: true
}
