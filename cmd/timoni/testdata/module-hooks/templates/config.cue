package templates

import (
	timoniv1 "timoni.sh/core/v1alpha1"
)

#Config: {
	kubeVersion!:   string
	clusterVersion: timoniv1.#SemVer & {#Version: kubeVersion, #Minimum: "1.20.0"}
	moduleVersion!: string
	metadata:       timoniv1.#Metadata & {#Version: moduleVersion}
}

#Instance: {
	config: #Config

	hooks: {
		sa: {
			apiVersion: "v1"
			kind:       "ServiceAccount"
			metadata: {
				name:      "\(config.metadata.name)-sa"
				namespace: config.metadata.namespace
				labels:    config.metadata.labels
				annotations: timoniv1.Action.Recreate
			}
		}
		dsn: {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: {
				name:      "\(config.metadata.name)-dsn"
				namespace: config.metadata.namespace
				labels:    config.metadata.labels
				annotations: timoniv1.Action.Recreate & timoniv1.Action.DeleteAfterHooks
			}
			data: {
				dsn: "postgres://localhost/app"
			}
		}
		migrate: {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: {
				name:      "\(config.metadata.name)-migrate"
				namespace: config.metadata.namespace
				labels:    config.metadata.labels
				annotations: timoniv1.Action.Recreate & timoniv1.Action.DeleteAfterHooks
			}
			data: {
				step: "migrate-up"
			}
		}
	}

	app: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: {
			name:      "\(config.metadata.name)-app"
			namespace: config.metadata.namespace
			labels:    config.metadata.labels
		}
		data: {
			role: "app"
		}
	}

	post: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: {
			name:      "\(config.metadata.name)-post"
			namespace: config.metadata.namespace
			labels:    config.metadata.labels
			annotations: timoniv1.Action.Recreate
		}
		data: {
			role: "post"
		}
	}
}
