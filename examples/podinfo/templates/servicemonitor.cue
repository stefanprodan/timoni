package templates

import (
	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
)

#ServiceMonitor: promv1.#ServiceMonitor & {
	_config:    #Config
	apiVersion: "monitoring.coreos.com/v1"
	kind:       promv1.#ServiceMonitorsKind
	metadata:   _config.metadata
	spec: {
		endpoints: [{
			path:     "/metrics"
			port:     "http-metrics"
			interval: _config.monitoring.interval
		}]
		namespaceSelector: matchNames: [_config.metadata.namespace]
		selector: matchLabels: _config.selectorLabels
	}
}
