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
	"maps"
	"os"

	"cuelang.org/go/cue/cuecontext"
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/flags"
	"github.com/stefanprodan/timoni/internal/runtime"
)

var bundleLintCmd = &cobra.Command{
	Use:   "lint",
	Short: "Validate bundle definitions",
	Long: `The bundle lint command validates that a bundle definition conforms with Timoni's schema.'.
`,
	Example: `  # Validate a bundle
  timoni bundle lint -f bundle.cue

  # Validate a bundle defined in multiple files
  timoni bundle lint \
  -f ./bundle.cue \
  -f ./bundle_secrets.cue
`,
	RunE: runBundleLintCmd,
}

type bundleLintFlags struct {
	pkg            flags.Package
	files          []string
	runtimeFromEnv bool
	runtimeFiles   []string
}

var bundleLintArgs bundleLintFlags

func init() {
	bundleLintCmd.Flags().VarP(&bundleLintArgs.pkg, bundleLintArgs.pkg.Type(), bundleLintArgs.pkg.Shorthand(), bundleLintArgs.pkg.Description())
	bundleLintCmd.Flags().StringSliceVarP(&bundleLintArgs.files, "file", "f", nil,
		"The local path to bundle.cue files.")
	bundleLintCmd.Flags().BoolVar(&bundleLintArgs.runtimeFromEnv, "runtime-from-env", false,
		"Inject runtime values from the environment.")
	bundleLintCmd.Flags().StringSliceVarP(&bundleLintArgs.runtimeFiles, "runtime", "r", nil,
		"The local path to runtime.cue files.")
	bundleCmd.AddCommand(bundleLintCmd)
}

func runBundleLintCmd(cmd *cobra.Command, args []string) error {
	log := LoggerFrom(cmd.Context())
	files := bundleLintArgs.files

	tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	cuectx := cuecontext.New()
	bm := engine.NewBundleBuilder(cuectx, files)

	runtimeValues := make(map[string]string)

	if bundleLintArgs.runtimeFromEnv {
		maps.Copy(runtimeValues, engine.GetEnv())
	}

	if len(bundleLintArgs.runtimeFiles) > 0 {
		kctx, cancel := context.WithTimeout(cmd.Context(), rootArgs.timeout)
		defer cancel()

		rt, err := buildRuntime(bundleLintArgs.runtimeFiles)
		if err != nil {
			return err
		}

		rm, err := runtime.NewResourceManager(kubeconfigArgs)
		if err != nil {
			return err
		}

		reader := runtime.NewResourceReader(rm)
		rv, err := reader.Read(kctx, rt.Refs)
		if err != nil {
			return err
		}

		maps.Copy(runtimeValues, rv)
	}

	if err := bm.InitWorkspace(tmpDir, runtimeValues); err != nil {
		return describeErr(tmpDir, "failed to parse bundle", err)
	}

	v, err := bm.Build()
	if err != nil {
		return describeErr(tmpDir, "failed to build bundle", err)
	}

	bundle, err := bm.GetBundle(v)
	if err != nil {
		return err
	}
	log = LoggerBundle(logr.NewContext(cmd.Context(), log), bundle.Name)

	if len(bundle.Instances) == 0 {
		return fmt.Errorf("no instances found in bundle")
	}

	for _, i := range bundle.Instances {
		if i.Namespace == "" {
			return fmt.Errorf("instance %s does not have a namespace", i.Name)
		}
		log := LoggerBundleInstance(logr.NewContext(cmd.Context(), log), bundle.Name, i.Name)
		log.Info("instance is valid")
	}

	log.Info("bundle is valid")
	return nil
}
