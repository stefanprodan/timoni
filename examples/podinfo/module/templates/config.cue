package templates

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"
)

#Config: {
	meta:           metav1.#ObjectMeta
	hpa:            #hpaConfig
	ingress:        #ingressConfig
	service:        #serviceConfig
	serviceMonitor: #serviceMonConfig

	image: {
		repository: *"ghcr.io/stefanprodan/podinfo" | string
		pullPolicy: *"IfNotPresent" | string
		tag:        string
	}

	cache?: string & =~"^tcp://"
	backends: [...string]
	logLevel: *"info" | string
	replicas: *1 | int

	selectorLabels: *{"app.kubernetes.io/name": meta.name} | {[ string]: string}
	meta: labels:      *selectorLabels | {[ string]:  string}
	meta: labels: "app.kubernetes.io/version": "\(image.tag)"
	meta: annotations?: {[ string]: string}

	resources?: corev1.#ResourceRequirements
	securityContext?: corev1.#PodSecurityContext
	affinity?:        corev1.#Affinity
	tolerations?: [ ...corev1.#Toleration]
}
