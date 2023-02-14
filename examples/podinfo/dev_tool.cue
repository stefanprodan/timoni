package main

import (
	"tool/cli"
	"tool/exec"
	"encoding/yaml"
	"text/tabwriter"
)

command: build: {
	task: print: cli.Print & {
		text: yaml.MarshalStream(output)
	}
}

command: ls: {
	task: print: cli.Print & {
		text: tabwriter.Write([
			"RESOURCE \tAPI VERSION",
			for r in output {
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
			"-u",
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
}

command: crds: {
	go_k8s: exec.Run & {
		cmd: [
			"go",
			"get",
			"-u",
			"github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1",
		]
	}
	cue_k8s: exec.Run & {
		$after: go_k8s
		cmd: [
			"cue",
			"get",
			"go",
			"github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1",
		]
	}
}

command: lint: {
	fmt: exec.Run & {
		cmd: [
			"cue",
			"fmt",
			"./...",
		]
	}
	vet: exec.Run & {
		$after: fmt
		cmd: [
			"cue",
			"vet",
			"-c",
			"./...",
		]
	}
}
