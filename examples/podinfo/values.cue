// The values.cue file is required by Timoni and should contain the user-facing defualt values.
// Note that this file must have no imports and all values must be concrete.

package main

values: {
	image: {
		repository: "ghcr.io/stefanprodan/podinfo"
		tag:        "6.3.4"
	}

	autoscaling: enabled: false
	monitoring: enabled:  false
	ingress: enabled:     false
}
