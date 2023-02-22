package main

import (
	"tool/cli"
	"encoding/yaml"
	"text/tabwriter"
)

command: build: {
	task: print: cli.Print & {
		text: yaml.MarshalStream(timoni.objects)
	}
}

command: ls: {
	task: print: cli.Print & {
		text: tabwriter.Write([
			"RESOURCE \tAPI VERSION",
			for r in timoni.objects {
				if r.metadata.namespace == _|_ {
					"\(r.kind)/\(r.metadata.name) \t\(r.apiVersion)"
				}
				if r.metadata.namespace != _|_ {
					"\(r.kind)/\(r.metadata.namespace)/\(r.metadata.name)  \t\(r.apiVersion)"
				}
			},
		])
	}
}
