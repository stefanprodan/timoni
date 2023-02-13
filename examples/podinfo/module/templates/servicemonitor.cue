package templates

import (
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
)

#serviceMonConfig: {
	enabled:  *false | bool
	interval: *"15s" | string
}

#ServiceMonitor: promv1.#ServiceMonitor & {
	_config:    #Config
	apiVersion: "monitoring.coreos.com/v1"
	kind:       promv1.#ServiceMonitorsKind
	metadata:   _config.meta
	spec: {
		endpoints: [{
			path:     "/metrics"
			port:     "http-metrics"
			interval: _config.serviceMonitor.interval
		}]
		namespaceSelector: matchNames: [_config.meta.namespace]
		selector: matchLabels: _config.selectorLabels
	}
}
