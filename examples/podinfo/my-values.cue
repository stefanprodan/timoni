values: {
	meta: {
		name:      "podinfo"
		namespace: "default"
		annotations: "app.kubernetes.io/owner": "dev-team"
	}
	image: tag: "6.3.3"
	resources: requests: {
		cpu:    "500m"
		memory: "256Mi"
	}
	hpa: {
		enabled:     true
		minReplicas: 2
		maxReplicas: 10
	}
	ingress: {
		enabled:   true
		className: "nginx"
		host:      "podinfo.example.com"
		tls:       true
		annotations: "cert-manager.io/cluster-issuer": "letsencrypt"
	}
	serviceMonitor: enabled: true
}
