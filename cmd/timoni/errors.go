package main

import (
	"fmt"

	"cuelang.org/go/cue/errors"
)

func describeErr(moduleRoot, description string, err error) error {
	return fmt.Errorf("%s:\n%s", description, errors.Details(err, &errors.Config{
		Cwd: moduleRoot,
	}))
}
