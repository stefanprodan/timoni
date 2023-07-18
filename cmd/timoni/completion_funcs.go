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
	"strings"

	"github.com/spf13/cobra"
	"github.com/stefanprodan/timoni/internal/runtime"
)

// completeNamespaceList completes a Cobra argument or flag with
// a Timoni instance, based on the current context in ~/.kube/config,
// and the current namespace set via --namespace.
func completeInstanceList(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	instances, err := listInstancesFromFlags()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var completions []string
	for _, inst := range instances {
		if strings.HasPrefix(inst.Name, toComplete) {
			completions = append(completions, inst.Name)
		}
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}

// completeNamespaceList completes a Cobra argument or flag with
// a Kubernetes namespace, based on the current context in ~/.kube/config
func completeNamespaceList(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	sm, err := runtime.NewResourceManager(kubeconfigArgs)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	iStorage := runtime.NewStorageManager(sm)

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	namespaces, err := iStorage.ListNamespaces(ctx)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	var completions []string
	for _, ns := range namespaces {
		if strings.HasPrefix(ns, toComplete) {
			completions = append(completions, ns)
		}
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}
