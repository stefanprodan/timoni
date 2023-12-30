# Kubernetes Custom Resources

Timoni allows defining Kubernetes Custom Resources (CRs) in modules and can ensure
that these are validated against their Kubernetes Custom Resource Definitions (CRDs).

To enable validation for custom resources, you have to generate the CUE schemas from
the Kubernetes CRDs OpenAPI validation spec with the `timoni mod vendor crds` command.

## Example

To demonstrate this feature, we'll use the
[Prometheus Operator](https://github.com/prometheus-operator/prometheus-operator) CRDs,
and we'll add a `ServiceMonitor` custom resource to a Timoni module.

### Vendor Prometheus Operator CRDs

From the root dir of your module, run the `timoni mod vendor crds` command, and pass the URL
to the YAML file which contains the Prometheus Operator CRDs:

```shell
timoni mod vendor crds -f https://github.com/prometheus-operator/prometheus-operator/releases/download/v0.68.0/stripped-down-crds.yaml
```

The above command will generate the CUE schemas corresponding to the Kubernetes CRDs
inside the `cue.mod/gen` directory:

```text
cue.mod/gen/
└── monitoring.coreos.com
    ├── alertmanager
    ├── alertmanagerconfig
    ├── podmonitor
    ├── probe
    ├── prometheus
    ├── prometheusagent
    ├── prometheusrule
    ├── scrapeconfig
    ├── servicemonitor
    └── thanosruler
```

### Create the `ServiceMonitor` template

In the `templates` directory, create a `servicemonitor.cue` file with the following content:

```cue
package templates

import (
	promv1 "monitoring.coreos.com/servicemonitor/v1"
)

#ServiceMonitor: promv1.#ServiceMonitor & {
	#config:  #Config
	metadata: #config.metadata
	spec: {
		endpoints: [{
			// Change this to match the Service port where
			// your app exposes the /metrics endpoint
			port:     "http-metrics"
			path:     "/metrics"
			interval: "\(#config.monitoring.interval)s"
		}]
		namespaceSelector: matchNames: [#config.metadata.namespace]
		selector: matchLabels: #config.selector.labels
	}
}
```

Make sure to replace the `port` and `path` values with the ones used by your app.
The port name must match one of the ports exposed in the Kubernetes Service template.

!!! tip "API Version and Kind"
    
    Note that for Kubernetes custom resources, you don't need to specify the 
    `apiVersion` and `kind`, these fields are set by Timoni in the generated schema.

### Add the `monitoring` configuration

In the `templates/config.cue` file, add the `monitoring` configuration:

```cue
#Config: {

	// Promethues service monitor (optional)
	monitoring: {
		enabled:  *false | bool
		interval: *15 | int & >=5 & <=3600
	}

}

```

### Add the `ServiceMonitor` to the instance

In the `templates/config.cue` file, add the `ServiceMonitor` resource to the instance objects:

```cue
#Instance: {
	config: #Config

	if config.monitoring.enabled {
		objects: servicemonitor: #ServiceMonitor & {#config: config}
	}

}

```

### Document the `monitoring` configuration

Finally, document the `monitoring` configuration in the `README.md` file, so that users
know how to enable monitoring if they have Prometheus Operator installed.
