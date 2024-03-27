/*
Copyright 2024 Stefan Prodan

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
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"cuelang.org/go/cue/cuecontext"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/engine/fetcher"
	"github.com/stefanprodan/timoni/internal/flags"
)

var configShowModCmd = &cobra.Command{
	Use:   "config [MODULE PATH]",
	Short: "Output the #Config structure of a local module",
	Long:  `The config command parses the local module configuration structure and outputs the information to stdout.`,
	Example: `  # print the config of a module in the current directory
  timoni mod show config

  # output the config to a file, if the file is markdown, the table will overwrite a table in a Configuration section or
  # be appended to the end of the file
  timoni mod show config --output ./README.md
`,
	RunE: runConfigShowModCmd,
}

type configModFlags struct {
	path   string
	pkg    flags.Package
	name   string
	output string
}

var configShowModArgs = configModFlags{
	name: "module-name",
}

func init() {
	configShowModCmd.Flags().StringVarP(&configShowModArgs.output, "output", "o", "", "The file to output the config Markdown to, defaults to stdout")
	showModCmd.AddCommand(configShowModCmd)
}

func runConfigShowModCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		configShowModArgs.path = "."
	} else {
		configShowModArgs.path = args[0]
	}

	if fs, err := os.Stat(configShowModArgs.path); err != nil || !fs.IsDir() {
		return fmt.Errorf("module not found at path %s", configShowModArgs.path)
	}

	cuectx := cuecontext.New()

	tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	ctxPull, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	f, err := fetcher.New(ctxPull, fetcher.Options{
		Source:       configShowModArgs.path,
		Version:      apiv1.LatestVersion,
		Destination:  tmpDir,
		CacheDir:     rootArgs.cacheDir,
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

	builder := engine.NewModuleBuilder(
		cuectx,
		configShowModArgs.name,
		*kubeconfigArgs.Namespace,
		f.GetModuleRoot(),
		configShowModArgs.pkg.String(),
	)

	if err := builder.WriteSchemaFile(); err != nil {
		return err
	}

	mod.Name, err = builder.GetModuleName()
	if err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	rows, err := builder.GetConfigDoc()

	if err != nil {
		return describeErr(f.GetModuleRoot(), "failed to get config structure", err)
	}

	header := []string{"Key", "Type", "Description"}

	if configShowModArgs.output == "" {
		printMarkDownTable(rootCmd.OutOrStdout(), header, rows)
	} else {
		tmpFile, err := writeFile(configShowModArgs.output, header, rows, f)
		if err != nil {
			return err
		}

		err = os.Rename(tmpFile, configShowModArgs.output)
		if err != nil {
			return describeErr(f.GetModuleRoot(), "Unable to rename file", err)
		}
	}

	return nil
}

func writeFile(readFile string, header []string, rows [][]string, f fetcher.Fetcher) (string, error) {
	// Generate the markdown table
	var tableBuffer bytes.Buffer
	tableWriter := bufio.NewWriter(&tableBuffer)
	printMarkDownTable(tableWriter, header, rows)
	tableWriter.Flush()
	// get a temporary file name
	tmpFileName := readFile + ".tmp"
	// open the input file
	inputFile, err := os.Open(readFile)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			inputFile, err = os.Create(readFile)

			if err != nil {
				return "", describeErr(f.GetModuleRoot(), "Unable to create the temporary output file", err)
			}
		} else {
			return "", describeErr(f.GetModuleRoot(), "Unable to create the temporary output file", err)
		}
	}
	defer inputFile.Close()

	// open the output file
	outputFile, err := os.Create(tmpFileName)
	if err != nil {
		return "", describeErr(f.GetModuleRoot(), "Unable to create the temporary output file", err)
	}
	defer outputFile.Close()

	// Create the scanner and writer
	inputScanner := bufio.NewScanner(inputFile)
	outputWriter := bufio.NewWriter(outputFile)
	var configSection bool
	var foundTable bool

	// Scan the input file line by line to find the table and replace it or append it to the end
	for inputScanner.Scan() {
		line := inputScanner.Text()

		if isMarkdownFile(readFile) {
			if !configSection && line == "## Configuration" {
				configSection = true
			}

			matched, err := regexp.MatchString(`^\|.*\|$`, line)
			if err != nil {
				return "", describeErr(f.GetModuleRoot(), "Regex Match for table content failed", err)
			}

			if configSection && !foundTable && matched {
				foundTable = true
				outputWriter.WriteString(tableBuffer.String() + "\n")
			} else if configSection && foundTable && matched {
			} else if configSection && foundTable && !matched {
				configSection = false
			} else {
				outputWriter.WriteString(line + "\n")
			}
		} else {
			outputWriter.WriteString(line + "\n")
		}
	}

	// If no table was found, append it to the end of the file
	if !foundTable {
		outputWriter.WriteString("\n" + tableBuffer.String())
	}

	err = outputWriter.Flush()
	if err != nil {
		return "", describeErr(f.GetModuleRoot(), "Failed to Flush Writer", err)
	}

	return tmpFileName, nil
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

func isMarkdownFile(filename string) bool {
	extension := strings.ToLower(filepath.Ext(filename))
	return extension == ".md" || extension == ".markdown"
}
