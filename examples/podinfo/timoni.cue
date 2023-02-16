// The timoni.cue file is required and should export
// a stream of Kubernetes objects under the timoni.objects field.

package main

import (
	templates "timoni.sh/podinfo/templates"
)

// Generate the Kubernetes objects by passing the name, namespace
// and the config values to the templates instance.
instance: templates.#Instance & {
	// The user-supplied values are merged with the
	// default values at runtime by Timoni.
	config: values
	// The instance name and namespace tag values
	// are injected at runtime by Timoni.
	config: metadata: {
		name:      string @tag(name)
		namespace: string @tag(namespace)
	}
}

// Expose the instance build result for Timoni's runtime.
timoni: {
	objects: [ for obj in instance.objects {obj}]
}
