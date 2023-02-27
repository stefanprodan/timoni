package templates

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"
)

// Config defines the schema and defaults for the Instance values.
#Config: {
	// Metadata (common to all resources)
	metadata: metav1.#ObjectMeta
	metadata: name:      *"podinfo" | string
	metadata: namespace: *"default" | string
	metadata: labels:    *selectorLabels | {[ string]: string}
	metadata: labels: "app.kubernetes.io/version": image.tag
	metadata: annotations?: {[ string]:            string}

	// Deployment
	replicas:       *1 | int & >0
	selectorLabels: *{"app.kubernetes.io/name": metadata.name} | {[ string]: string}

	// Pod
	podAnnotations?: {[ string]: string}
	podSecurityContext?: corev1.#PodSecurityContext
	imagePullSecrets?: [...corev1.LocalObjectReference]
	tolerations?: [ ...corev1.#Toleration]
	affinity?: corev1.#Affinity
	topologySpreadConstraints?: [...corev1.#TopologySpreadConstraint]

	// Container
	image: {
		repository: *"ghcr.io/stefanprodan/podinfo" | string
		tag:        *"6.3.4" | string
		pullPolicy: *"IfNotPresent" | string
	}
	resources?:       corev1.#ResourceRequirements
	securityContext?: corev1.#SecurityContext

	// Service
	service: port: *80 | int & >0 & <=65535

	// HorizontalPodAutoscaler (optional)
	autoscaling: {
		enabled:     *false | bool
		cpu:         *99 | int & >0 & <=100
		memory:      *"" | string
		minReplicas: *replicas | int
		maxReplicas: *minReplicas | int & >=minReplicas
	}

	// Ingress (optional)
	ingress: {
		enabled: *false | bool
		tls:     *false | bool
		host:    *"podinfo.local" | string
		annotations?: {[ string]: string}
		className?: string
	}

	// ServiceMonitor (optional)
	monitoring: {
		enabled:  *false | bool
		interval: *"15s" | string
	}
}

// Instance takes the config values and outputs the Kubernetes objects.
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
