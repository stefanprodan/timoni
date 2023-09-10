package templates

#Config: {
	team!: string

	metadata: {
		name:      *"test" | string
		namespace: *"default" | string
		labels:    *{
				"app.kubernetes.io/name":    metadata.name
				"app.kubernetes.io/version": moduleVersion
				"app.kubernetes.io/kube":    kubeVersion
				"app.kubernetes.io/team":    team
		} | {[ string]: string}
		annotations?: {[ string]: string}
	}

	moduleVersion: string
	kubeVersion:   string

	client: enabled: *true | bool
	server: enabled: *true | bool
	domain: *"example.internal" | string

	ns: enabled: *false | bool
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
