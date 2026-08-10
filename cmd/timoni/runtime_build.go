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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"cuelang.org/go/cue/cuecontext"
	"github.com/spf13/cobra"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/logger"
	"github.com/stefanprodan/timoni/internal/runtime"
)

var runtimeBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build validates the runtime definition, queries the cluster, extracts the values and prints them",
	Example: `  #  Print the runtime values from a cluster
  timoni runtime build -f runtime.cue
`,
	Args: cobra.NoArgs,
	RunE: runRuntimeBuildCmd,
}

type runtimeBuildFlags struct {
	files                []string
	clusterSelector      string
	clusterGroupSelector string
	workdir              string
}

var runtimeBuildArgs runtimeBuildFlags

func init() {
	runtimeBuildCmd.Flags().StringSliceVarP(&runtimeBuildArgs.files, "file", "f", nil,
		"The local path to runtime.cue files.")
	runtimeBuildCmd.Flags().StringVar(&runtimeBuildArgs.clusterSelector, "cluster", "*",
		"Select cluster by name.")
	runtimeBuildCmd.Flags().StringVar(&runtimeBuildArgs.clusterGroupSelector, "cluster-group", "*",
		"Select clusters by group name.")
	runtimeBuildCmd.Flags().StringVar(&runtimeBuildArgs.workdir, "workdir", "",
		"The local path to the CUE module root (the directory containing cue.mod), used to resolve imports in the runtime definitions. Defaults to the current directory.")
	runtimeCmd.AddCommand(runtimeBuildCmd)
}

func runRuntimeBuildCmd(cmd *cobra.Command, args []string) error {
	files := runtimeBuildArgs.files
	if len(files) == 0 {
		return errors.New("no runtime provided with -f")
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

	rt, err := buildRuntime(files, runtimeBuildArgs.workdir)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), rootArgs.timeout)
	defer cancel()

	clusters := rt.SelectClusters(runtimeBuildArgs.clusterSelector, runtimeBuildArgs.clusterGroupSelector)
	if len(clusters) == 0 {
		return errors.New("no cluster found")
	}

	for _, cluster := range clusters {
		log := loggerRuntime(cmd.Context(), rt.Name, cluster.Name, true)

		kubeconfigArgs.Context = &cluster.KubeContext
		rm, err := runtime.NewResourceManager(kubeconfigArgs)
		if err != nil {
			return err
		}

		reader := runtime.NewResourceReader(rm)

		values, err := reader.Read(ctx, rt.Refs)
		if err != nil {
			return err
		}

		keys := make([]string, 0, len(values))

		for k := range values {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			log.Info(fmt.Sprintf("%s: %s", logger.ColorizeSubject(k), values[k]))
		}

		if len(values) == 0 {
			log.Info("no values defined")
		}
	}

	return nil
}

// buildRuntime compiles the runtime definitions into a Runtime object.
// The workdir is the CUE module root used to resolve imports; when empty,
// the CUE loader falls back to the process working directory.
func buildRuntime(files []string, workdir string) (*apiv1.Runtime, error) {
	defaultRuntime := apiv1.DefaultRuntime(*kubeconfigArgs.Context)
	if len(files) == 0 {
		return defaultRuntime, nil
	}

	dir, err := resolveWorkdir(workdir)
	if err != nil {
		return nil, err
	}

	ctx := cuecontext.New()
	rb := engine.NewRuntimeBuilder(ctx, files)
	rb.SetWorkdir(dir)

	workspace := apiv1.RuntimeDefaultName
	if err := rb.InitWorkspace(workspace); err != nil {
		return nil, describeErr(rb.WorkspaceDir(workspace), "failed to init runtime", err)
	}

	v, err := rb.Build(workspace)
	if err != nil {
		return nil, describeErr(rb.WorkspaceDir(workspace), "failed to parse runtime", err)
	}

	rt, err := rb.GetRuntime(v)
	if err != nil {
		return nil, err
	}

	if len(rt.Clusters) == 0 {
		rt.Clusters = defaultRuntime.Clusters
	}
	return rt, nil
}

// resolveWorkdir validates that the given workdir exists and is a
// directory, and returns its absolute path. An empty workdir resolves
// to empty, letting the CUE loader use the process working directory.
func resolveWorkdir(workdir string) (string, error) {
	if workdir == "" {
		return "", nil
	}

	dir, err := filepath.Abs(workdir)
	if err != nil {
		return "", fmt.Errorf("invalid workdir %s: %w", workdir, err)
	}

	fi, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("invalid workdir: %w", err)
	}
	if !fi.IsDir() {
		return "", fmt.Errorf("invalid workdir %s: not a directory", dir)
	}

	return dir, nil
}
