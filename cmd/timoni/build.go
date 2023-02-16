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
)

var buildCmd = &cobra.Command{
	Use:     "build [INSTANCE NAME] [MODULE URL]",
	Aliases: []string{"template"},
	Short:   "Build a module and print the resulting Kubernetes resources",
	Example: `  # Build a local module with the default values
  timoni build app ./path/to/module --output yaml

  # Build a module with custom values by merging them in the specified order. 
  timoni build app ./path/to/module \
  --values ./values-1.cue \
  --values ./values-2.cue
`,
	RunE: runBuildCmd,
}

type buildFlags struct {
	name        string
	module      string
	version     string
	pkg         string
	valuesFiles []string
	output      string
	creds       string
}

var buildArgs buildFlags

func init() {
	buildCmd.Flags().StringVarP(&buildArgs.version, "version", "v", "",
		"version of the module.")
	buildCmd.Flags().StringVarP(&buildArgs.pkg, "package", "p", "main",
		"The name of the package containing the instance values and resources.")
	buildCmd.Flags().StringSliceVarP(&buildArgs.valuesFiles, "values", "f", nil,
		"local path to values.cue files")
	buildCmd.Flags().StringVarP(&buildArgs.output, "output", "o", "yaml",
		"the format in which the Kubernetes resources should be printed, can be 'json' or 'yaml'")
	buildCmd.Flags().StringVar(&buildArgs.creds, "creds", "",
		"credentials for the container registry in the format <username>[:<password>]")

	rootCmd.AddCommand(buildCmd)
}

func runBuildCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("name and module are required")
	}

	buildArgs.name = args[0]
	buildArgs.module = args[1]

	ctx := cuecontext.New()

	tmpDir, err := os.MkdirTemp("", "timoni")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	ctxPull, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	fetcher := NewFetcher(ctxPull, buildArgs.module, buildArgs.version, tmpDir, buildArgs.creds)
	modulePath, err := fetcher.Fetch()
	if err != nil {
		return err
	}

	builder := NewBuilder(ctx, buildArgs.name, *kubeconfigArgs.Namespace, modulePath, buildArgs.pkg)

	if len(buildArgs.valuesFiles) > 0 {
		err = builder.MergeValuesFile(buildArgs.valuesFiles)
		if err != nil {
			return err
		}
	}

	buildResult, err := builder.Build()
	if err != nil {
		return fmt.Errorf("failed to build instance, error: %w", err)
	}

	objects, err := builder.GetObjects(buildResult)
	if err != nil {
		return fmt.Errorf("failed to extract Kubernetes objects, error: %w", err)
	}
	switch buildArgs.output {
	case "yaml":
		var sb strings.Builder
		for _, obj := range objects {
			data, err := yaml.Marshal(obj)
			if err != nil {
				return fmt.Errorf("failed to convert resouces, error: %w", err)
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
			return fmt.Errorf("failed to convert resouces, error: %w", err)
		}
		_, err = cmd.OutOrStdout().Write(b)
	default:
		return fmt.Errorf("unkown --output=%s, can be yaml or json", buildArgs.output)
	}

	return err
}
