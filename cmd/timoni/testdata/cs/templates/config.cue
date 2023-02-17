package templates

#Config: {
	metadata: {
		name:      *"test" | string
		namespace: *"default" | string
		labels:    *{"app.kubernetes.io/name": metadata.name} | {[ string]: string}
		annotations?: {[ string]: string}
	}

	client: enabled: *true | bool
	server: enabled: *true | bool
	domain: string
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
	}
}
