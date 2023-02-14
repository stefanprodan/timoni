// The output.cue file is required by Tiomni and should export
// a stream of Kubernetes objects under the output field.

package main

import (
	templates "podinfo.mod/templates"
)

// Generate the Kubernetes objects by passing the config values
// to the templates instance.
instance: templates.#Instance & {
	config: values
}

// Set the output with the generted Kubernetes objects.
output: [ for obj in instance.objects {obj}]
