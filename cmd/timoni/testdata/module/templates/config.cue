package templates

import (
	timoniv1 "timoni.sh/core/v1alpha1"
)

#Config: {
	moduleVersion!: string
	kubeVersion!:   string

	// Common metadata for all objects
	metadata: timoniv1.#Metadata & {#Version: moduleVersion}
	metadata: labels: {
		// +nodoc
		"app.kubernetes.io/kube": kubeVersion
		// +nodoc
		"app.kubernetes.io/team": team
	}

	client: {
		enabled: *true | bool

		image: timoniv1.#Image & {
			repository: *"cgr.dev/chainguard/timoni" | string
			tag:        *"latest-dev" | string
			digest:     *"sha256:b49fbaac0eedc22c1cfcd26684707179cccbed0df205171bae3e1bae61326a10" | string
		}
	}

	server: {
		enabled: *true | bool
	}
	domain: *"example.internal" | string

	ns: {
		enabled: *false | bool
	}

	team!: string
}

#Instance: {
	config: #Config

	objects: {
		if config.client.enabled {
			"\(config.metadata.name)-client": #ClientConfig & {_config: config}
		}

		if config.server.enabled {
			"\(config.metadata.name)-server": #ServerConfig & {_config: config}
		}

		if config.ns.enabled {
			"\(config.metadata.name)-ns": #Namespace & {_config: config}
		}
	}
}
