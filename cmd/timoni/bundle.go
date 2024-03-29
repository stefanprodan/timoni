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
	"github.com/spf13/cobra"
)

type bundleFlags struct {
	runtimeFromEnv      bool
	runtimeFiles        []string
	runtimeCluster      string
	runtimeClusterGroup string
}

var bundleArgs bundleFlags

var bundleCmd = &cobra.Command{
	Use:   "bundle",
	Short: "Commands for managing bundles",
}

func init() {
	bundleCmd.PersistentFlags().BoolVar(&bundleArgs.runtimeFromEnv, "runtime-from-env", false,
		"Inject runtime values from the environment.")
	bundleCmd.PersistentFlags().StringSliceVarP(&bundleArgs.runtimeFiles, "runtime", "r", nil,
		"The local path to runtime.cue files.")
	bundleCmd.PersistentFlags().StringVar(&bundleArgs.runtimeCluster, "runtime-cluster", "*",
		"Filter runtime cluster by name.")
	bundleCmd.PersistentFlags().StringVar(&bundleArgs.runtimeClusterGroup, "runtime-group", "*",
		"Filter runtime clusters by group.")
	rootCmd.AddCommand(bundleCmd)
}
