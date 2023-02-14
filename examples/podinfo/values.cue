// The values.cue file is required by Timoni and should contain the user-facing defualt values.
// Note that this file must have no imports and all values must be concrete.

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
