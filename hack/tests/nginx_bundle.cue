bundle: {
	apiVersion: "v1alpha1"
	name:       "nginx"
	instances: {
		"nginx-local": {
			module: {
				url: "file://../../blueprints/starter"
			}
			namespace: "nginx"
			values: {
				resources: limits: {
					cpu:    "100m"
					memory: "128Mi"
				}
			}
		}
		"nginx-remote": {
			module: {
				url: string @timoni(runtime:string:NGINX_MODULE_URL)
			}
			namespace: "nginx"
			values: {
				resources: limits: memory: "128Mi"
			}
		}
	}
}
