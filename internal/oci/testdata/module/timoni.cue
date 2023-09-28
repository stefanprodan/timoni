package main

import (
	templates "timoni.sh/test/templates"
)

values: templates.#Config

timoni: {
	apiVersion: "v1alpha1"
	instance: templates.#Instance & {
		config: values
		config: metadata: {
			name:      string @tag(name)
			namespace: string @tag(namespace)
		}
		config: {
			moduleVersion: string @tag(mv, var=moduleVersion)
			kubeVersion:   string @tag(kv, var=kubeVersion)
		}
	}

	apply: all: [ for obj in instance.objects {obj}]
}
