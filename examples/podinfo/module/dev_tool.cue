package main

import (
	"tool/cli"
	"tool/exec"
	"encoding/yaml"
	"text/tabwriter"
)

command: gen: {
	task: print: cli.Print & {
		text: yaml.MarshalStream(resources)
	}
}

command: ls: {
	task: print: cli.Print & {
		text: tabwriter.Write([
			"RESOURCE \tAPI VERSION",
			for r in resources {
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

command: apis: {
	go_k8s: exec.Run & {
		cmd: [
			"go",
			"get",
			"k8s.io/api/...",
		]
	}
	cue_k8s: exec.Run & {
		$after: go_k8s
		cmd: [
			"cue",
			"get",
			"go",
			"k8s.io/api/...",
		]
	}
	go_prom: exec.Run & {
		$after: cue_k8s
		cmd: [
			"go",
			"get",
			"github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1",
		]
	}
	cue_prom: exec.Run & {
		$after: go_k8s
		cmd: [
			"cue",
			"get",
			"go",
			"github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1",
		]
	}
}
