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
	"os"
	"time"

	"github.com/fluxcd/pkg/oci"
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

var (
	VERSION     = "0.0.0-dev.0"
	CUE_VERSION = "0.5.0"
)

var rootCmd = &cobra.Command{
	Use:           "timoni",
	Version:       VERSION,
	SilenceUsage:  true,
	SilenceErrors: true,
	Short:         "A package manager for Kubernetes powered by CUE.",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Initialize the console logger just before running
		// a command only if one wasn't provided. This allows other
		// callers (e.g. unit tests) to inject their own logger ahead of time.
		if logger.IsZero() {
			logger = NewConsoleLogger(rootArgs.prettyLog)
		}

		// Inject the logger in the command context.
		ctx := logr.NewContext(context.Background(), logger)
		cmd.SetContext(ctx)
	},
}

type rootFlags struct {
	timeout   time.Duration
	prettyLog bool
}

var (
	rootArgs       = rootFlags{}
	logger         logr.Logger
	kubeconfigArgs = genericclioptions.NewConfigFlags(false)
)

func init() {
	rootCmd.PersistentFlags().DurationVar(&rootArgs.timeout, "timeout", 5*time.Minute,
		"The length of time to wait before giving up on the current operation.")
	rootCmd.PersistentFlags().BoolVar(&rootArgs.prettyLog, "log-pretty", true,
		"Adds timestamps and colorized output to the logs.")

	addKubeConfigFlags(rootCmd)

	rootCmd.DisableAutoGenTag = true
	rootCmd.SetOut(os.Stdout)

	oci.UserAgent = apiv1.UserAgent
	oci.CanonicalConfigMediaType = apiv1.ConfigMediaType
	oci.CanonicalContentMediaType = apiv1.ContentMediaType
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		// Ensure a logger is initialized even if the rootCmd
		// failed before running its hooks.
		if logger.IsZero() {
			logger = NewConsoleLogger(rootArgs.prettyLog)
		}

		// Set the logger err to nil to pretty print
		// the error message on multiple lines.
		logger.Error(nil, err.Error())
		os.Exit(1)
	}
}

// addKubeConfigFlags maps the kubectl config flags to the given persistent flags.
// The default namespace is set to the value found in current kubeconfig context.
func addKubeConfigFlags(cmd *cobra.Command) {
	kubeconfigArgs.Timeout = nil
	kubeconfigArgs.Namespace = nil
	kubeconfigArgs.AddFlags(cmd.PersistentFlags())

	namespace := "default"

	// Try to read the default namespace from the current context.
	if ns, _, err := kubeconfigArgs.ToRawKubeConfigLoader().Namespace(); err == nil {
		namespace = ns
	}

	kubeconfigArgs.Namespace = &namespace

	cmd.PersistentFlags().StringVarP(kubeconfigArgs.Namespace, "namespace", "n", *kubeconfigArgs.Namespace, "The instance namespace.")
	cmd.RegisterFlagCompletionFunc("namespace", completeNamespaceList)
}
