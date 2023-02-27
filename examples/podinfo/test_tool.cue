package main

import (
	"tool/cli"
	"encoding/yaml"
	"text/tabwriter"
)

// The build command generates the Kubernetes manifests and prints the multi-docs YAML to stdout.
// Run 'cue -t test build' to use the values from test_values.cue.
command: build: {
	task: print: cli.Print & {
		text: yaml.MarshalStream(timoni.apply.all)
	}
}

// The ls command prints a table with the Kubernetes resources kind, namespace, name and version.
// Run 'cue -t test ls' to use the values from test_values.cue.
command: ls: {
	task: print: cli.Print & {
		text: tabwriter.Write([
			"RESOURCE \tAPI VERSION",
			for r in timoni.apply.all {
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
