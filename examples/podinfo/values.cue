// The values.cue is required by Tiomni and should contains the default values.
// Note that this file will be overriten by Tiomni during build time,
// so only concreate values are allowed.

package main

values: {
	metadata: {
		name:      "podinfo"
		namespace: "default"
	}

	image: tag: "6.3.3"

	autoscaling: enabled: false
	monitoring: enabled:  false
	ingress: enabled:     false
}
