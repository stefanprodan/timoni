package templates

cases: "service defaults to ClusterIP on port 80": {
	objects: "Service/test/test": {
		apiVersion: "v1"
		metadata: labels: "app.kubernetes.io/managed-by": "timoni"
		spec: {
			type: "ClusterIP"
			selector: "app.kubernetes.io/name": "test"
			ports: [{name: "http", port: 80, protocol: "TCP", targetPort: "http"}]
		}
	}
}

cases: "service port is configurable": {
	values: service: port: 9898
	objects: "Service/test/test": spec: ports: [{port: 9898}]
}

cases: "service annotations are propagated": {
	values: service: annotations: "external-dns.alpha.kubernetes.io/hostname": "app.example.com"
	objects: "Service/test/test": metadata: annotations: "external-dns.alpha.kubernetes.io/hostname": "app.example.com"
}

cases: "service has no annotations by default": {
	objects: "Service/test/test": kind: "Service"
	assert: "annotations are absent": objects["Service/test/test"].metadata.annotations == _|_
}

cases: "service selector targets the deployment pods": {
	objects: {
		"Service/test/test": kind:    "Service"
		"Deployment/test/test": kind: "Deployment"
	}
	assert: "selector matches the pod template labels":
		objects["Service/test/test"].spec.selector ==
		objects["Deployment/test/test"].spec.template.metadata.labels
}

cases: "service port outside the valid range is rejected": {
	_config: #Config & {service: port: 70000}
	assert: "port 70000 is rejected": _config == _|_
}
