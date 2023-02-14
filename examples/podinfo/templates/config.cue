package templates

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"
)

#Config: {
	metadata: metav1.#ObjectMeta
	metadata: name:      string
	metadata: namespace: string
	selectorLabels: *{"app.kubernetes.io/name": metadata.name} | {[ string]: string}
	metadata: labels: *selectorLabels | {[ string]: string}
	metadata: labels: "app.kubernetes.io/version": image.tag
	metadata: annotations?: {[ string]:            string}

	image: {
		repository: *"ghcr.io/stefanprodan/podinfo" | string
		pullPolicy: *"IfNotPresent" | string
		tag:        string
	}

	replicas: *1 | int
	autoscaling: {
		enabled:     *false | bool
		cpu:         *99 | int | >0 & <=100
		memory:      *"" | string
		minReplicas: *replicas | int
		maxReplicas: *replicas | int
	}

	service: port: *80 | int
	ingress: {
		enabled: *false | bool
		tls:     *false | bool
		host:    *"podinfo.local" | string
		annotations?: {[ string]: string}
		className?: string
	}

	monitoring: {
		enabled:  *false | bool
		interval: *"15s" | string
	}

	resources?:       corev1.#ResourceRequirements
	securityContext?: corev1.#SecurityContext
	affinity?:        corev1.#Affinity
	tolerations?: [ ...corev1.#Toleration]
}

#Instance: {
	config: #Config

	objects: {
		"\(config.metadata.name)-sa":     #ServiceAccount & {_config: config}
		"\(config.metadata.name)-svc":    #Service & {_config:        config}
		"\(config.metadata.name)-deploy": #Deployment & {_config:     config}

		if config.autoscaling.enabled {
			"\(config.metadata.name)-hpa": #HorizontalPodAutoscaler & {_config: config}
		}

		if config.ingress.enabled {
			"\(config.metadata.name)-ingress": #Ingress & {_config: config}
		}

		if config.monitoring.enabled {
			"\(config.metadata.name)-monitor": #ServiceMonitor & {_config: config}
		}
	}
}
