@extern(embed)

package templates

import (
	"text/template"

	timoniv1 "timoni.sh/core/v1alpha1"
)

// The config files are embedded from the sibling files at build time
// and rendered with the instance config using Go text templating.
_nginxConf: string @embed(file="nginx.default.conf", type=text)
_indexHTML: string @embed(file="index.html", type=text)

#ConfigMap: timoniv1.#ImmutableConfig & {
	#config: #Config
	#Kind:   timoniv1.#ConfigMapKind
	#Meta:   #config.metadata
	#Data: {
		"nginx.default.conf": template.Execute(_nginxConf, #config)
		"index.html":         template.Execute(_indexHTML, #config)
	}
}
