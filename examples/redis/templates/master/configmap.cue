@extern(embed)

package master

import (
	"text/template"

	timoniv1 "timoni.sh/core/v1alpha1"
	"timoni.sh/redis/templates/config"
)

// The Redis configuration is embedded from the sibling redis.conf file
// at build time and rendered with the instance config.
_redisConf: string @embed(file="redis.conf", type=text)

#ConfigMap: timoniv1.#ImmutableConfig & {
	#config: config.#Config
	#Kind:   timoniv1.#ConfigMapKind
	#Meta:   #config.metadata
	#Data: "redis.conf": template.Execute(_redisConf, #config)
}
