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
	"fmt"
	"io"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/stefanprodan/timoni/pkg/inventory"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Prints a table of module instances",
	Example: ` # List the module instances installed in a namespace
  timoni list --namespace default

  # List the module instances in all namespaces
  timoni list -A
`,
	RunE: runListCmd,
}

type listFlags struct {
	allNamespaces bool
}

var listArgs listFlags

func init() {
	listCmd.Flags().BoolVarP(&listArgs.allNamespaces, "all-namespaces", "A", false,
		"list the requested object(s) across all namespaces.")

	rootCmd.AddCommand(listCmd)
}

func runListCmd(cmd *cobra.Command, args []string) error {
	sm, err := newManager(owner)
	if err != nil {
		return err
	}

	invStorage := &inventory.Storage{Manager: sm, Owner: owner}

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	ns := *kubeconfigArgs.Namespace
	if listArgs.allNamespaces {
		ns = ""
	}
	inventories, err := invStorage.ListInventories(ctx, ns)
	if err != nil {
		return err
	}

	var rows [][]string
	for _, inv := range inventories {
		row := []string{}
		if listArgs.allNamespaces {
			row = []string{inv.Name, inv.Namespace, fmt.Sprintf("%v", len(inv.Resources)), inv.Source, inv.Revision, inv.LastAppliedAt}
		} else {
			row = []string{inv.Name, fmt.Sprintf("%v", len(inv.Resources)), inv.Source, inv.Revision, inv.LastAppliedAt}
		}
		rows = append(rows, row)
	}

	if listArgs.allNamespaces {
		printTable(rootCmd.OutOrStdout(), []string{"name", "namespace", "entries", "source", "version", "last applied"}, rows)
	} else {
		printTable(rootCmd.OutOrStdout(), []string{"name", "entries", "source", "version", "last applied"}, rows)
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
