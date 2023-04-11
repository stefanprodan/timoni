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
	"fmt"
	"os"

	"cuelang.org/go/cue/cuecontext"
	"github.com/spf13/cobra"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/flags"
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
	pkg   flags.Package
	files []string
}

var bundleLintArgs bundleLintFlags

func init() {
	bundleLintCmd.Flags().VarP(&bundleLintArgs.pkg, bundleLintArgs.pkg.Type(), bundleLintArgs.pkg.Shorthand(), bundleLintArgs.pkg.Description())
	bundleLintCmd.Flags().StringSliceVarP(&bundleLintArgs.files, "file", "f", nil,
		"The local path to bundle.cue files.")
	bundleCmd.AddCommand(bundleLintCmd)
}

func runBundleLintCmd(cmd *cobra.Command, args []string) error {
	bundleSchema, err := os.CreateTemp("", "schema.*.cue")
	if err != nil {
		return err
	}
	defer os.Remove(bundleSchema.Name())
	if _, err := bundleSchema.WriteString(apiv1.BundleSchema); err != nil {
		return err
	}
	files := append(bundleLintArgs.files, bundleSchema.Name())

	ctx := cuecontext.New()
	bm := engine.NewBundleBuilder(ctx, files)

	v, err := bm.Build()
	if err != nil {
		return err
	}

	instances, err := bm.GetInstances(v)
	if err != nil {
		return err
	}

	if len(instances) == 0 {
		return fmt.Errorf("no instances found in bundle")
	}

	for _, i := range instances {
		if i.Namespace == "" {
			return fmt.Errorf("instance %s does not have a namespace", i.Name)
		}
		logger.Printf("instance %s is valid", i.Name)
	}

	logger.Printf("bundle is valid")
	return nil
}
