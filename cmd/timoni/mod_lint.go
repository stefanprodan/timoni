/*
Copyright 2023 Stefan Prodan

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"fmt"
	"os"

	"cuelang.org/go/cue/cuecontext"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/flags"
)

var lintModCmd = &cobra.Command{
	Use:   "lint [MODULE PATH]",
	Short: "Validate a local module",
	Long:  `The lint command builds the local module and validates the resulting Kubernetes objects.`,
	Example: `  # lint a local module
  timoni mod lint ./path/to/module
`,
	RunE: runLintModCmd,
}

type lintModFlags struct {
	path string
	pkg  flags.Package
}

var lintModArgs lintModFlags

func init() {
	lintModCmd.Flags().VarP(&lintModArgs.pkg, lintModArgs.pkg.Type(), lintModArgs.pkg.Shorthand(), lintModArgs.pkg.Description())

	modCmd.AddCommand(lintModCmd)
}

func runLintModCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("module path is required")
	}

	lintModArgs.path = args[0]
	if fs, err := os.Stat(lintModArgs.path); err != nil || !fs.IsDir() {
		return fmt.Errorf("module not found at path %s", lintModArgs.path)
	}

	log := LoggerFrom(cmd.Context())
	cuectx := cuecontext.New()

	tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	ctxPull, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	fetcher := engine.NewFetcher(
		ctxPull,
		lintModArgs.path,
		apiv1.LatestVersion,
		tmpDir,
		"",
	)
	mod, err := fetcher.Fetch()
	if err != nil {
		return err
	}

	builder := engine.NewModuleBuilder(
		cuectx,
		"default",
		*kubeconfigArgs.Namespace,
		fetcher.GetModuleRoot(),
		lintModArgs.pkg.String(),
	)

	if err := builder.WriteSchemaFile(); err != nil {
		return err
	}

	mod.Name, err = builder.GetModuleName()
	if err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	buildResult, err := builder.Build()
	if err != nil {
		return describeErr(fetcher.GetModuleRoot(), "build failed", err)
	}

	if _, err := builder.GetConfigValues(buildResult); err != nil {
		return fmt.Errorf("failed to extract values: %w", err)
	}

	applySets, err := builder.GetApplySets(buildResult)
	if err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	if len(applySets) == 0 {
		return fmt.Errorf("%s contains no objects", apiv1.ApplySelector)
	}

	var objects []*unstructured.Unstructured
	for _, set := range applySets {
		objects = append(objects, set.Objects...)
	}

	if len(objects) == 0 {
		return fmt.Errorf("build failed, no objects to apply")
	}

	log.Info(fmt.Sprintf("%s linted", mod.Name))

	return nil
}
