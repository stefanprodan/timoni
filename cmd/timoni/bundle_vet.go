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

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/flags"
	"github.com/stefanprodan/timoni/internal/runtime"
)

var bundleVetCmd = &cobra.Command{
	Use:     "vet",
	Aliases: []string{"lint"},
	Short:   "Validate a bundle definition",
	Long: `The bundle vet command validates that a bundle definition conforms
with Timoni's schema and optionally prints the computed value.
`,
	Example: `  # Validate a bundle and list its instances
  timoni bundle vet -f bundle.cue

  # Validate a bundle defined in multiple files and print the computed value
  timoni bundle vet \
  -f ./bundle.cue \
  -f ./bundle_secrets.cue \
  --print-value

  # Validate a bundle with runtime attributes and print the computed value
  timoni bundle vet \
  -f bundle.cue \
  -r runtime.cue \
  --print-value
`,
	Args: cobra.NoArgs,
	RunE: runBundleVetCmd,
}

type bundleVetFlags struct {
	pkg            flags.Package
	files          []string
	runtimeFromEnv bool
	runtimeFiles   []string
	printValue     bool
}

var bundleVetArgs bundleVetFlags

func init() {
	bundleVetCmd.Flags().VarP(&bundleVetArgs.pkg, bundleVetArgs.pkg.Type(), bundleVetArgs.pkg.Shorthand(), bundleVetArgs.pkg.Description())
	bundleVetCmd.Flags().StringSliceVarP(&bundleVetArgs.files, "file", "f", nil,
		"The local path to bundle.cue files.")
	bundleVetCmd.Flags().BoolVar(&bundleVetArgs.runtimeFromEnv, "runtime-from-env", false,
		"Inject runtime values from the environment.")
	bundleVetCmd.Flags().StringSliceVarP(&bundleVetArgs.runtimeFiles, "runtime", "r", nil,
		"The local path to runtime.cue files.")
	bundleVetCmd.Flags().BoolVar(&bundleVetArgs.printValue, "print-value", false,
		"Print the computed value of the bundle.")
	bundleCmd.AddCommand(bundleVetCmd)
}

func runBundleVetCmd(cmd *cobra.Command, args []string) error {
	log := LoggerFrom(cmd.Context())
	files := bundleVetArgs.files
	if len(files) == 0 {
		return fmt.Errorf("no bundle provided with -f")
	}
	var stdinFile string
	for i, file := range files {
		if file == "-" {
			stdinFile, err := saveReaderToFile(cmd.InOrStdin())
			if err != nil {
				return err
			}
			files[i] = stdinFile
			break
		}
	}
	if stdinFile != "" {
		defer os.Remove(stdinFile)
	}

	tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	cuectx := cuecontext.New()
	bm := engine.NewBundleBuilder(cuectx, files)

	runtimeValues := make(map[string]string)

	if bundleVetArgs.runtimeFromEnv {
		maps.Copy(runtimeValues, engine.GetEnv())
	}

	if len(bundleVetArgs.runtimeFiles) > 0 {
		kctx, cancel := context.WithTimeout(cmd.Context(), rootArgs.timeout)
		defer cancel()

		rt, err := buildRuntime(bundleVetArgs.runtimeFiles)
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
	log = LoggerBundle(logr.NewContext(cmd.Context(), log), bundle.Name, apiv1.RuntimeDefaultName)

	if len(bundle.Instances) == 0 {
		return fmt.Errorf("no instances found in bundle")
	}

	if bundleVetArgs.printValue {
		val := v.LookupPath(cue.ParsePath("bundle"))
		if val.Err() != nil {
			return err
		}
		_, err := rootCmd.OutOrStdout().Write([]byte(fmt.Sprintf("bundle: %v\n", val)))
		return err
	}

	for _, i := range bundle.Instances {
		if i.Namespace == "" {
			return fmt.Errorf("instance %s does not have a namespace", i.Name)
		}
		log := LoggerBundleInstance(logr.NewContext(cmd.Context(), log), bundle.Name, apiv1.RuntimeDefaultName, i.Name)
		log.Info("instance is valid")
	}

	log.Info("bundle is valid")
	return nil
}
