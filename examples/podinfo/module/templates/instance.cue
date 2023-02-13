package templates

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

#Instance: {
	Config: #Config
	Objects: [ID=_]: runtime.#Object

	Objects: {
		service:    #Service & {_config:        Config}
		account:    #ServiceAccount & {_config: Config}
		deployment: #Deployment & {
			_config:         Config
			_serviceAccount: account.metadata.name
		}
	}

	if Config.hpa.enabled == true {
		Objects: hpa: #HorizontalPodAutoscaler & {_config: Config}
	}

	if Config.ingress.enabled == true {
		Objects: ingress: #Ingress & {_config: Config}
	}

	if Config.serviceMonitor.enabled == true {
		Objects: serviceMonitor: #ServiceMonitor & {_config: Config}
	}
}
