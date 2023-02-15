// The timoni.cue file is required and should export
// a stream of Kubernetes objects under the timoni.objects field.

package main

import (
	templates "timoni.sh/podinfo/templates"
)

timoni: {
	name:      *"podinfo" | string @tag(name)
	namespace: *"default" | string @tag(namespace)
	objects: [ for obj in instance.objects {obj}]
}

// Generate the Kubernetes objects by passing the name, namespace
// and the config values to the templates instance.
instance: templates.#Instance & {
	config: values
	config: metadata: {
		name:      timoni.name
		namespace: timoni.namespace
	}
}
