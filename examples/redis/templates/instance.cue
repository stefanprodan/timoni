package templates

import (
	c "timoni.sh/redis/templates/config"
	m "timoni.sh/redis/templates/master"
	r "timoni.sh/redis/templates/replica"
)

#Instance: {
	config: c.#Config

	// The immutable ConfigMap is shared by the master and replica deployments.
	// Its generated name is passed to the deployments so that a config change
	// triggers a rolling update.
	_cm: m.#ConfigMap & {#config: config}

	master: objects: {
		"\(config.metadata.name)-sa": m.#ServiceAccount & {#config: config}
		"\(config.metadata.name)-cm": _cm

		if config.persistence.enabled {
			"\(config.metadata.name)-pvc": m.#MasterPVC & {#config: config}
		}

		"\(config.metadata.name)-svc": m.#MasterService & {#config: config}
		"\(config.metadata.name)-deploy": m.#MasterDeployment & {
			#config: config
			#cmName: _cm.metadata.name
		}
	}

	replica: objects: {
		"\(config.metadata.name)-deploy-replica": r.#ReplicaDeployment & {
			#config: config
			#cmName: _cm.metadata.name
		}
		"\(config.metadata.name)-svc-replica": r.#ReplicaService & {#config: config}
	}

	test: objects: {
		"\(config.metadata.name)-ping-master": m.#TestJob & {#config: config}
	}
}
