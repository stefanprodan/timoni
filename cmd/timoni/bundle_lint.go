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

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
	"github.com/spf13/cobra"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
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

	ctx := cuecontext.New()

	cfg := &load.Config{
		Package:   "_",
		DataFiles: true,
	}

	files := append(bundleLintArgs.files, bundleSchema.Name())
	ix := load.Instances(files, cfg)
	if len(ix) == 0 {
		return fmt.Errorf("no bundle found")
	}

	inst := ix[0]
	if inst.Err != nil {
		return fmt.Errorf("bundle error: %w", inst.Err)
	}

	v := ctx.BuildInstance(inst)
	if v.Err() != nil {
		return v.Err()
	}

	if err := v.Validate(cue.Concrete(true)); err != nil {
		return err
	}

	apiVersion := v.LookupPath(cue.ParsePath(apiv1.BundleAPIVersionSelector.String()))
	if apiVersion.Err() != nil {
		return fmt.Errorf("lookup %s failed, error: %w", apiv1.BundleAPIVersionSelector.String(), apiVersion.Err())
	}

	apiVer, _ := apiVersion.String()
	if apiVer != apiv1.GroupVersion.Version {
		return fmt.Errorf("API version %s not supported, must be %s", apiVer, apiv1.GroupVersion.Version)
	}

	instances := v.LookupPath(cue.ParsePath(apiv1.BundleInstancesSelector.String()))
	if instances.Err() != nil {
		return fmt.Errorf("lookup %s failed, error: %w", apiv1.BundleInstancesSelector.String(), instances.Err())
	}

	var instCount int
	iter, _ := instances.Fields(cue.Concrete(true))
	for iter.Next() {
		name := iter.Selector().String()
		expr := iter.Value()

		namespace := expr.LookupPath(cue.ParsePath(apiv1.BundleNamespaceSelector.String()))
		if namespace.Err() != nil {
			return fmt.Errorf("lookup %s failed, error: %w", apiv1.BundleNamespaceSelector.String(), instances.Err())
		}

		if _, err := namespace.String(); err != nil {
			return fmt.Errorf("invalid %s, error: %w", apiv1.BundleNamespaceSelector.String(), err)
		}

		logger.Printf("instance %s is valid", name)
		instCount++
	}

	if instCount == 0 {
		return fmt.Errorf("no instances found in bundle")
	}

	logger.Printf("bundle is valid")
	return nil
}
