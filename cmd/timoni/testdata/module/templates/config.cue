package templates

import (
	timoniv1 "timoni.sh/core/v1alpha1"
)

#Config: {
	// +nodoc
	kubeVersion!: string
	// +nodoc
	clusterVersion: timoniv1.#SemVer & {#Version: kubeVersion, #Minimum: "1.20.0"}
	// +nodoc
	moduleVersion!: string

	// Common metadata for all objects
	metadata: timoniv1.#Metadata & {#Version: moduleVersion}

	// +nodoc
	metadata: labels: {
		// +nodoc
		"app.kubernetes.io/kube": kubeVersion
		// +nodoc
		"app.kubernetes.io/team": team
	}

	// +nodoc
	client: {
		enabled: *true | bool

		// +nodoc
		image: timoniv1.#Image & {
			repository: *"cgr.dev/chainguard/timoni" | string
			tag:        *"latest-dev" | string
			digest:     *"sha256:b49fbaac0eedc22c1cfcd26684707179cccbed0df205171bae3e1bae61326a10" | string
		}
	}

	// +nodoc
	server: {
		enabled: *true | bool
	}
	domain: *"example.internal" | string

	// +nodoc
	ns: {
		enabled: *false | bool
	}

	team!: string
}

#Instance: {
	config: #Config

	objects: {
		if config.client.enabled {
			"\(config.metadata.name)-client": #ClientConfig & {#config: config}
		}

		if config.server.enabled {
			"\(config.metadata.name)-server": #ServerConfig & {#config: config}
		}

		if config.ns.enabled {
			"\(config.metadata.name)-ns": #Namespace & {#config: config}
		}
	}
}
