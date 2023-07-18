package templates

import (
	"encoding/yaml"
	"strings"
	"uuid"

	corev1 "k8s.io/api/core/v1"
)

#ConfigMap: corev1.#ConfigMap & {
	_config:    #Config
	apiVersion: "v1"
	kind:       "ConfigMap"
	let _data_sha = strings.Split(uuid.SHA1(uuid.ns.DNS, yaml.Marshal(data)), "-")[0]
	metadata: {
		name:         "\(_config.metadata.name)-\(_data_sha)"
		namespace:    _config.metadata.namespace
		labels:       _config.metadata.labels
		annotations?: _config.metadata.annotations
	}
	immutable: true
	data:
		"index.html": """
			<!doctype html>
			<html lang="en">
			<head>
			 	<meta charset="utf-8">
			 	<meta http-equiv="refresh" content="5" />
				<title>\(_config.metadata.name)</title>
				<style>
				html { color-scheme: light dark; }
				body { width: 35em; margin: 0 auto;
				font-family: Tahoma, Verdana, Arial, sans-serif; }
				</style>
			</head>
			<body>
				<h1> \(_config.message) from \(_config.metadata.name)!</h1>
				<p>If you see this page, the <b>\(_config.metadata.name)</b> instance is successfully deployed in the <b>\(_config.metadata.namespace)</b> namespace by Timoni.</p>
			</body>
			</html>
			"""
}
