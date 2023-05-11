package main

import (
	"tool/cli"
)

command: test: {
	task: print: cli.Print & {
		text: "test"
	}
}
