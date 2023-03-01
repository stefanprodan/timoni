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
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"cuelang.org/go/cue/cuecontext"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/flags"
)

var buildCmd = &cobra.Command{
	Use:     "build [INSTANCE NAME] [MODULE URL]",
	Aliases: []string{"template"},
	Short:   "Build an instance from a module and print the resulting Kubernetes resources",
	Example: `  # Build an instance from a local module
  timoni build app ./path/to/module --output yaml

  # Build an instance with custom values by merging them in the specified order
  timoni build app ./path/to/module \
  --values ./values-1.cue \
  --values ./values-2.cue
`,
	RunE: runBuildCmd,
}

type buildFlags struct {
	name        string
	module      string
	version     flags.Version
	pkg         flags.Package
	valuesFiles []string
	output      string
	creds       flags.Credentials
}

var buildArgs buildFlags

func init() {
	buildCmd.Flags().VarP(&buildArgs.version, buildArgs.version.Type(), buildArgs.version.Shorthand(), buildArgs.version.Description())
	buildCmd.Flags().VarP(&buildArgs.pkg, buildArgs.pkg.Type(), buildArgs.pkg.Shorthand(), buildArgs.pkg.Description())
	buildCmd.Flags().StringSliceVarP(&buildArgs.valuesFiles, "values", "f", nil,
		"The local path to values.cue files.")
	buildCmd.Flags().StringVarP(&buildArgs.output, "output", "o", "yaml",
		"The format in which the Kubernetes objects should be printed, can be 'yaml' or 'yaml'.")
	buildCmd.Flags().Var(&buildArgs.creds, buildArgs.creds.Type(), buildArgs.creds.Description())

	rootCmd.AddCommand(buildCmd)
}

func runBuildCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("name and module are required")
	}

	buildArgs.name = args[0]
	buildArgs.module = args[1]

	version := buildArgs.version.String()
	if version == "" {
		version = engine.LatestTag
	}

	ctx := cuecontext.New()

	tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	ctxPull, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	fetcher := engine.NewFetcher(
		ctxPull,
		buildArgs.module,
		version,
		tmpDir,
		buildArgs.creds.String(),
	)
	mod, err := fetcher.Fetch()
	if err != nil {
		return err
	}

	builder := engine.NewModuleBuilder(
		ctx,
		buildArgs.name,
		*kubeconfigArgs.Namespace,
		fetcher.GetModuleRoot(),
		buildArgs.pkg.String(),
	)

	mod.Name, err = builder.GetModuleName()
	if err != nil {
		return err
	}

	if len(buildArgs.valuesFiles) > 0 {
		err = builder.MergeValuesFile(buildArgs.valuesFiles)
		if err != nil {
			return err
		}
	}

	buildResult, err := builder.Build()
	if err != nil {
		return fmt.Errorf("build failed, error: %w", err)
	}

	apiVer, err := builder.GetAPIVersion(buildResult)
	if err != nil {
		return err
	}

	if apiVer != apiv1.GroupVersion.Version {
		return fmt.Errorf("API version %s not supported, must be %s", apiVer, apiv1.GroupVersion.Version)
	}

	applySets, err := builder.GetApplySets(buildResult)
	if err != nil {
		return fmt.Errorf("failed to extract objects, error: %w", err)
	}

	var objects []*unstructured.Unstructured
	for _, set := range applySets {
		objects = append(objects, set.Objects...)
	}

	switch buildArgs.output {
	case "yaml":
		var sb strings.Builder
		for _, obj := range objects {
			data, err := yaml.Marshal(obj)
			if err != nil {
				return fmt.Errorf("converting objects failed, error: %w", err)
			}
			sb.Write(data)
			sb.WriteString("---\n")
		}
		_, err = cmd.OutOrStdout().Write([]byte(sb.String()))
	case "json":
		list := struct {
			ApiVersion string                       `json:"apiVersion,omitempty"`
			Kind       string                       `json:"kind,omitempty"`
			Items      []*unstructured.Unstructured `json:"items,omitempty"`
		}{
			ApiVersion: "v1",
			Kind:       "List",
			Items:      objects,
		}

		b, err := json.MarshalIndent(list, "", "    ")
		if err != nil {
			return fmt.Errorf("converting objects failed, error: %w", err)
		}
		_, err = cmd.OutOrStdout().Write(b)
	default:
		return fmt.Errorf("unkown --output=%s, can be yaml or json", buildArgs.output)
	}

	return err
}
