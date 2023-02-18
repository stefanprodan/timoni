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
	"io"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/stefanprodan/timoni/internal/runtime"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "Prints a table of instances and their module version",
	Example: ` # List all instances in a namespace
  timoni list --namespace default

  # List all instances on a cluster
  timoni ls -A
`,
	RunE: runListCmd,
}

type listFlags struct {
	allNamespaces bool
}

var listArgs listFlags

func init() {
	listCmd.Flags().BoolVarP(&listArgs.allNamespaces, "all-namespaces", "A", false,
		"List the requested object(s) across all namespaces.")

	rootCmd.AddCommand(listCmd)
}

func runListCmd(cmd *cobra.Command, args []string) error {
	sm, err := runtime.NewResourceManager(kubeconfigArgs)
	if err != nil {
		return err
	}

	iStorage := runtime.NewStorageManager(sm)

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	ns := *kubeconfigArgs.Namespace
	if listArgs.allNamespaces {
		ns = ""
	}

	instances, err := iStorage.List(ctx, ns)
	if err != nil {
		return err
	}

	var rows [][]string
	for _, inv := range instances {
		row := []string{}
		if listArgs.allNamespaces {
			row = []string{
				inv.Name,
				inv.Namespace,
				inv.Module.Repository,
				inv.Module.Version,
				inv.LastTransitionTime,
			}
		} else {
			row = []string{
				inv.Name,
				inv.Module.Repository,
				inv.Module.Version,
				inv.LastTransitionTime,
			}
		}
		rows = append(rows, row)
	}

	if listArgs.allNamespaces {
		printTable(rootCmd.OutOrStdout(), []string{"name", "namespace", "module", "version", "last applied"}, rows)
	} else {
		printTable(rootCmd.OutOrStdout(), []string{"name", "module", "version", "last applied"}, rows)
	}

	return nil
}

func printTable(writer io.Writer, header []string, rows [][]string) {
	table := tablewriter.NewWriter(writer)
	table.SetHeader(header)
	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetHeaderLine(false)
	table.SetBorder(false)
	table.SetTablePadding("\t")
	table.SetNoWhiteSpace(true)
	table.AppendBulk(rows)
	table.Render()
}
