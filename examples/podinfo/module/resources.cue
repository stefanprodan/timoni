package main

import (
	templates "podinfo.mod/templates"
)

values: templates.#Config

instance: templates.#Instance & {
	Config: values
}

// resources are required by Timoni to generate Kubernetes objects
resources: [ for obj in instance.Objects {obj}]
