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
	"os"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

var VERSION = "0.0.0-dev.0"

var rootCmd = &cobra.Command{
	Use:           "timoni",
	Version:       VERSION,
	SilenceUsage:  true,
	SilenceErrors: true,
	Short:         "A package manager for Kubernetes powered by CUE.",
}

type rootFlags struct {
	timeout time.Duration
}

var (
	rootArgs = rootFlags{}
	logger   = stderrLogger{stderr: os.Stderr}
)

var kubeconfigArgs = genericclioptions.NewConfigFlags(false)

func init() {
	rootCmd.PersistentFlags().DurationVar(&rootArgs.timeout, "timeout", time.Minute,
		"The length of time to wait before giving up on the current operation.")

	kubeconfigArgs.Timeout = nil
	kubeconfigArgs.Namespace = nil
	kubeconfigArgs.AddFlags(rootCmd.PersistentFlags())

	defaultNamespace := "default"
	kubeconfigArgs.Namespace = &defaultNamespace
	rootCmd.PersistentFlags().StringVarP(kubeconfigArgs.Namespace, "namespace", "n", *kubeconfigArgs.Namespace, "The instance namespace.")

	rootCmd.DisableAutoGenTag = true
	rootCmd.SetOut(os.Stdout)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		logger.Println(`✗`, err)
		os.Exit(1)
	}
}
