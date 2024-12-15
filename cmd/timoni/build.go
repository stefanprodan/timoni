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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	cuejson "cuelang.org/go/encoding/json"
	cueyaml "cuelang.org/go/encoding/yaml"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/engine/fetcher"
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
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		switch len(args) {
		case 0:
			return completeInstanceList(cmd, args, toComplete)
		case 1:
			return nil, cobra.ShellCompDirectiveFilterDirs
		default:
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
	},
}

type buildFlags struct {
	name        string
	module      string
	version     flags.Version
	pkg         flags.Package
	digest      flags.Digest
	valuesFiles []string
	output      string
	creds       flags.Credentials
}

var buildArgs buildFlags

func init() {
	buildCmd.Flags().VarP(&buildArgs.version, buildArgs.version.Type(), buildArgs.version.Shorthand(), buildArgs.version.Description())
	buildCmd.Flags().VarP(&buildArgs.pkg, buildArgs.pkg.Type(), buildArgs.pkg.Shorthand(), buildArgs.pkg.Description())
	buildCmd.Flags().VarP(&buildArgs.digest, buildArgs.digest.Type(), buildArgs.digest.Shorthand(), buildArgs.digest.Description())
	buildCmd.Flags().StringSliceVarP(&buildArgs.valuesFiles, "values", "f", nil,
		"The local path to values files (cue, yaml or json format).")
	buildCmd.Flags().StringVarP(&buildArgs.output, "output", "o", "yaml",
		"The format in which the Kubernetes objects should be printed, can be 'yaml' or 'json'.")
	buildCmd.Flags().Var(&buildArgs.creds, buildArgs.creds.Type(), buildArgs.creds.Description())

	rootCmd.AddCommand(buildCmd)
}

func runBuildCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 2 {
		return errors.New("name and module are required")
	}

	buildArgs.name = args[0]
	buildArgs.module = args[1]

	version := buildArgs.version.String()
	digest := buildArgs.digest.String()
	if version == "" {
		version = apiv1.LatestVersion
		if digest != "" {
			version = fmt.Sprintf("@%s", digest)
		}
	}

	ctx := cuecontext.New()

	tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	ctxPull, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	f, err := fetcher.New(ctxPull, fetcher.Options{
		Source:       buildArgs.module,
		Version:      version,
		Destination:  tmpDir,
		CacheDir:     rootArgs.cacheDir,
		Creds:        buildArgs.creds.String(),
		Insecure:     rootArgs.registryInsecure,
		DefaultLocal: true,
	})
	if err != nil {
		return err
	}
	mod, err := f.Fetch()
	if err != nil {
		return err
	}

	if digest != "" && mod.Digest != digest {
		return fmt.Errorf("digest mismatch, expected %s got %s", digest, mod.Digest)
	}

	builder := engine.NewModuleBuilder(
		ctx,
		buildArgs.name,
		*kubeconfigArgs.Namespace,
		f.GetModuleRoot(),
		buildArgs.pkg.String(),
	)

	if err := builder.WriteSchemaFile(); err != nil {
		return err
	}

	mod.Name, err = builder.GetModuleName()
	if err != nil {
		return err
	}

	if len(buildArgs.valuesFiles) > 0 {
		valuesCue, err := convertToCue(cmd, buildArgs.valuesFiles)
		if err != nil {
			return err
		}
		err = builder.MergeValuesFile(valuesCue)
		if err != nil {
			return err
		}
	}

	buildResult, err := builder.Build()
	if err != nil {
		return describeErr(f.GetModuleRoot(), "build failed", err)
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
		return fmt.Errorf("failed to extract objects: %w", err)
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
				return fmt.Errorf("converting objects failed: %w", err)
			}
			sb.Write(data)
			sb.WriteString("---\n")
		}
		_, err = cmd.OutOrStdout().Write([]byte(sb.String()))
		return err
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
			return fmt.Errorf("converting objects failed: %w", err)
		}
		_, err = cmd.OutOrStdout().Write(b)
		return err
	default:
		return fmt.Errorf("unknown --output=%s, can be yaml or json", buildArgs.output)
	}
}

func convertToCue(cmd *cobra.Command, paths []string) ([][]byte, error) {
	valuesCue := make([][]byte, len(paths))
	for i, path := range paths {
		var (
			bs  []byte
			err error
			ext string
		)

		if path == "-" {
			ext = ".cue"
			var buf bytes.Buffer
			_, err = io.Copy(&buf, cmd.InOrStdin())
			if err == nil {
				bs = buf.Bytes()
			}
		} else {
			ext = filepath.Ext(path)
			bs, err = os.ReadFile(path)
		}
		if err != nil {
			return nil, fmt.Errorf("could not read values file at %s: %w", path, err)
		}

		var node ast.Node

		switch ext {
		case ".cue":
			valuesCue[i] = bs
			continue
		case ".json":
			node, err = cuejson.Extract(path, bs)
			if err != nil {
				return nil, fmt.Errorf("could not extract JSON from %s: %w", path, err)
			}
		case ".yaml", ".yml":
			node, err = cueyaml.Extract(path, bs)
			if err != nil {
				return nil, fmt.Errorf("could not extract YAML from %s: %w", path, err)
			}
		default:
			return nil, fmt.Errorf("unknown values file format for %s", path)
		}

		bytes, err := format.Node(node)
		if err != nil {
			return nil, fmt.Errorf("could not serialise value from file at %s to cue: %w", path, err)
		}
		valuesCue[i] = bytes
	}
	return valuesCue, nil
}
