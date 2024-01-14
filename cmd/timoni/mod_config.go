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
	"os"

	"cuelang.org/go/cue/cuecontext"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/flags"
)

var configModCmd = &cobra.Command{
	Use:   "config [MODULE PATH]",
	Short: "Output the #Config structure of a local module",
	Long:  `The config command parses the local module configuration structure and outputs the information to stdout.`,
	Example: `  # print the config of a module in the current directory
  timoni mod config

  # print the config of a module in a specific directory
  timoni mod config ./path/to/module
`,
	RunE: runConfigModCmd,
}

type configModFlags struct {
	path string
	pkg  flags.Package
	name string
}

var configModArgs = configModFlags{
	name: "timoni",
}

func init() {
	modCmd.AddCommand(configModCmd)
}

func runConfigModCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		configModArgs.path = "."
	} else {
		configModArgs.path = args[0]
	}

	if fs, err := os.Stat(configModArgs.path); err != nil || !fs.IsDir() {
		return fmt.Errorf("module not found at path %s", configModArgs.path)
	}

	//log := LoggerFrom(cmd.Context())
	cuectx := cuecontext.New()

	tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	ctxPull, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	fetcher := engine.NewFetcher(
		ctxPull,
		configModArgs.path,
		apiv1.LatestVersion,
		tmpDir,
		rootArgs.cacheDir,
		"",
		rootArgs.registryInsecure,
	)
	mod, err := fetcher.Fetch()
	if err != nil {
		return err
	}

	builder := engine.NewModuleBuilder(
		cuectx,
		configModArgs.name,
		*kubeconfigArgs.Namespace,
		fetcher.GetModuleRoot(),
		configModArgs.pkg.String(),
	)

	if err := builder.WriteSchemaFile(); err != nil {
		return err
	}

	mod.Name, err = builder.GetModuleName()
	if err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	buildResult, err := builder.Build()
	if err != nil {
		return describeErr(fetcher.GetModuleRoot(), "validation failed", err)
	}

	rows, err := builder.GetConfigStructure(buildResult)
	if err != nil {
		return describeErr(fetcher.GetModuleRoot(), "failed to get config structure", err)
	}

	printMarkDownTable(os.Stdout, []string{"Field", "Type", "Default", "Description"}, rows)

	return nil
}

func printMarkDownTable(writer io.Writer, header []string, rows [][]string) {
	table := tablewriter.NewWriter(writer)
	table.SetHeader(header)
	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("|")
	table.SetColumnSeparator("|")
	table.SetRowSeparator("-")
	table.SetHeaderLine(true)
	table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
	table.SetTablePadding("\t")
	table.SetNoWhiteSpace(false)
	table.AppendBulk(rows)
	table.Render()
}
