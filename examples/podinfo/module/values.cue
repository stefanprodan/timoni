package main

// values are required by Timoni to be used as defaults
values: {
	meta: {
		name: "podinfo"
		namespace: "default"
	}
	image: tag: "6.3.3"
	resources: requests: {
		cpu:    "100m"
		memory: "128Mi"
	}
	resources: limits: {
		cpu:    2
		memory: "1Gi"
	}
}
