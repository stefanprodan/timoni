package main

import (
	templates "timoni.sh/test/templates"
)

output: (templates.#Instance & {
	config: values
	config: metadata: {
		name:      string @tag(name)
		namespace: string @tag(namespace)
	}
}).objects

timoni: {
	objects: [ for obj in output {obj}]
}
