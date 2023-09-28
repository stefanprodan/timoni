package templates

#Config: {
	moduleVersion: string
	kubeVersion:   string

	metadata: {
		name:      *"test" | string
		namespace: *"default" | string
	}

	hostname: *"default.internal" | string
}

#Instance: {
	config: #Config

	objects: {
		cm: #ConfigMap & {_config: config}
	}
}
