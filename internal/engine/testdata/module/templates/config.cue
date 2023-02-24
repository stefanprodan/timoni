package templates

#Config: {
	metadata: {
		name:      *"test" | string
		namespace: *"default" | string
	}
	hostname: string
}

#Instance: {
	config: #Config

	objects: {
		"\(config.metadata.name)": #KubeConfig & {_config: config}
	}
}
