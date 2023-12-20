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
	"path"
	"time"

	"github.com/fatih/color"
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

var (
	VERSION     = "0.0.0-dev.0"
	CUE_VERSION = "0.7.0"
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
			logger = NewConsoleLogger()
		}

		// Inject the logger in the command context.
		ctx := logr.NewContext(context.Background(), logger)
		cmd.SetContext(ctx)
	},
}

type rootFlags struct {
	timeout          time.Duration
	prettyLog        bool
	coloredLog       bool
	cacheDir         string
	registryInsecure bool
}

var (
	rootArgs = rootFlags{
		prettyLog:  true,
		coloredLog: !color.NoColor,
		timeout:    5 * time.Minute,
	}
	logger         logr.Logger
	kubeconfigArgs = genericclioptions.NewConfigFlags(false)
)

func init() {
	rootCmd.PersistentFlags().DurationVar(&rootArgs.timeout, "timeout", rootArgs.timeout,
		"The length of time to wait before giving up on the current operation.")
	rootCmd.PersistentFlags().BoolVar(&rootArgs.prettyLog, "log-pretty", rootArgs.prettyLog,
		"Adds timestamps to the logs.")
	rootCmd.PersistentFlags().BoolVar(&rootArgs.coloredLog, "log-color", rootArgs.coloredLog,
		"Adds colorized output to the logs. (defaults to false when no tty)")
	rootCmd.PersistentFlags().StringVar(&rootArgs.cacheDir, "cache-dir", "",
		"Artifacts cache dir, can be disable with 'TIMONI_CACHING=false' env var. (defaults to \"$HOME/.timoni/cache\")")
	rootCmd.PersistentFlags().BoolVar(&rootArgs.registryInsecure, "registry-insecure", false,
		"If true, allows connecting to a container registry without TLS or with a self-signed certificate.")

	addKubeConfigFlags(rootCmd)

	rootCmd.DisableAutoGenTag = true
	rootCmd.SetOut(color.Output)
	rootCmd.SetErr(color.Error)
}

func main() {
	setCacheDir()
	if err := rootCmd.Execute(); err != nil {
		// Ensure a logger is initialized even if the rootCmd
		// failed before running its hooks.
		if logger.IsZero() {
			logger = NewConsoleLogger()
		}

		// Set the logger err to nil to pretty print
		// the error message on multiple lines.
		logger.Error(nil, err.Error())
		os.Exit(1)
	}
}

func setCacheDir() {
	caching := os.Getenv("TIMONI_CACHING")
	if caching == "false" || caching == "0" {
		rootArgs.cacheDir = ""
		return
	}
	if rootArgs.cacheDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		rootArgs.cacheDir = path.Join(home, ".timoni/cache")
	}

	if err := os.MkdirAll(rootArgs.cacheDir, os.ModePerm); err != nil {
		// disable caching if target dir is not writable
		rootArgs.cacheDir = ""
	}
}

// addKubeConfigFlags maps the kubectl config flags to the given persistent flags.
// The default namespace is set to the value found in current kubeconfig context.
func addKubeConfigFlags(cmd *cobra.Command) {
	namespace := "default"
	// Try to read the default namespace from the current context.
	if ns, _, err := kubeconfigArgs.ToRawKubeConfigLoader().Namespace(); err == nil {
		namespace = ns
	}
	kubeconfigArgs.Namespace = &namespace

	cmd.PersistentFlags().StringVar(kubeconfigArgs.KubeConfig, "kubeconfig", os.Getenv("KUBECONFIG"), "Path to the kubeconfig file.")
	cmd.PersistentFlags().StringVar(kubeconfigArgs.Context, "kube-context", "", "The name of the kubeconfig context to use.")
	cmd.PersistentFlags().StringVar(kubeconfigArgs.Impersonate, "kube-as", "", "Username to impersonate for the operation. User could be a regular user or a service account in a namespace.")
	cmd.PersistentFlags().StringArrayVar(kubeconfigArgs.ImpersonateGroup, "kube-as-group", nil, "Group to impersonate for the operation, this flag can be repeated to specify multiple groups.")
	cmd.PersistentFlags().StringVar(kubeconfigArgs.ImpersonateUID, "kube-as-uid", "", "UID to impersonate for the operation.")
	cmd.PersistentFlags().StringVar(kubeconfigArgs.BearerToken, "kube-token", "", "Bearer token for authentication to the API server.")
	cmd.PersistentFlags().StringVar(kubeconfigArgs.APIServer, "kube-server", "", "The address and port of the Kubernetes API server.")
	cmd.PersistentFlags().StringVar(kubeconfigArgs.TLSServerName, "kube-tls-server-name", "", "Server name to use for server certificate validation. If it is not provided, the hostname used to contact the server is used.")
	cmd.PersistentFlags().StringVar(kubeconfigArgs.CertFile, "kube-client-certificate", "", "Path to a client certificate file for TLS.")
	cmd.PersistentFlags().StringVar(kubeconfigArgs.KeyFile, "kube-client-key", "", "Path to a client key file for TLS.")
	cmd.PersistentFlags().StringVar(kubeconfigArgs.CAFile, "kube-certificate-authority", "", "Path to a cert file for the certificate authority.")
	cmd.PersistentFlags().BoolVar(kubeconfigArgs.Insecure, "kube-insecure-skip-tls-verify", false, "if true, the Kubernetes API server's certificate will not be checked for validity. This will make your HTTPS connections insecure.")
	cmd.PersistentFlags().StringVarP(kubeconfigArgs.Namespace, "namespace", "n", *kubeconfigArgs.Namespace, "The the namespace scope for the operation.")
	cmd.RegisterFlagCompletionFunc("namespace", completeNamespaceList)
}
