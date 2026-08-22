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
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/spf13/cobra"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/engine/fetcher"
	"github.com/stefanprodan/timoni/internal/flags"
)

// markdownTableRowRegex matches a markdown table row.
var markdownTableRowRegex = regexp.MustCompile(`^\|.*\|$`)

var configShowModCmd = &cobra.Command{
	Use:   "config [MODULE PATH | MODULE URL]",
	Args:  cobra.MaximumNArgs(1),
	Short: "Output the #Config schema of a module",
	Long: `The config command prints the module's #Config schema as CUE, with the
field documentation, default values and constraints, marking optional fields
with ? and required fields with !. The module can be a local directory or
an OCI artifact.

With --output, the schema is written as a Markdown table to the given file.
If the file is Markdown, the table replaces the first table found under
the Configuration section, or is appended to the end of the file.`,
	Example: `  # print the config schema of a module in the current directory
  timoni mod show config

  # print the config schema of a module published to a container registry
  timoni mod show config oci://docker.io/org/app -v 1.0.0

  # write the config table to the module README
  timoni mod show config --output ./README.md
`,
	RunE: runConfigShowModCmd,
}

type configModFlags struct {
	path    string
	version flags.Version
	creds   flags.Credentials
	pkg     flags.Package
	name    string
	output  string
}

var configShowModArgs = configModFlags{
	name: "module-name",
}

func init() {
	configShowModCmd.Flags().VarP(&configShowModArgs.version, configShowModArgs.version.Type(), configShowModArgs.version.Shorthand(), configShowModArgs.version.Description())
	configShowModCmd.Flags().Var(&configShowModArgs.creds, configShowModArgs.creds.Type(), configShowModArgs.creds.Description())
	configShowModCmd.Flags().StringVarP(&configShowModArgs.output, "output", "o", "", "The file to output the config Markdown to, defaults to stdout")
	showModCmd.AddCommand(configShowModCmd)
}

func runConfigShowModCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		configShowModArgs.path = "."
	} else {
		configShowModArgs.path = args[0]
	}

	version := configShowModArgs.version.String()
	if version == "" {
		version = apiv1.LatestVersion
	}

	if configShowModArgs.output != "" && strings.HasPrefix(configShowModArgs.path, apiv1.ArtifactPrefix) {
		return fmt.Errorf("--output is not supported for OCI modules, the README is published with the artifact")
	}

	cuectx := cuecontext.New()

	tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	ctxPull, cancel := context.WithTimeout(cmd.Context(), rootArgs.timeout)
	defer cancel()

	f, err := fetcher.New(ctxPull, fetcher.Options{
		Source:       configShowModArgs.path,
		Version:      version,
		Destination:  tmpDir,
		CacheDir:     rootArgs.cacheDir,
		Creds:        configShowModArgs.creds.String(),
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

	if err := builder.OverlaySchemaFile(); err != nil {
		return err
	}

	mod.Name, err = builder.GetModuleName()
	if err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	buildResult, err := builder.Build()
	if err != nil {
		return describeErr(f.GetModuleRoot(), "validation failed", err)
	}

	fields, err := builder.GetConfigDoc(buildResult)
	if err != nil {
		return describeErr(f.GetModuleRoot(), "failed to get config structure", err)
	}

	if configShowModArgs.output == "" {
		out, err := engine.FormatConfigCUE(fields)
		if err != nil {
			return describeErr(f.GetModuleRoot(), "failed to format config", err)
		}
		_, err = fmt.Fprint(rootCmd.OutOrStdout(), out)
		return err
	}

	rows := configTableRows(fields)
	header := []string{"Key", "Type", "Default", "Description"}

	{
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

// writeFile updates the generated config table and returns the temporary output path.
// It reports input errors before the caller replaces the destination.
func writeFile(readFile string, header []string, rows [][]string, f fetcher.Fetcher) (string, error) {
	// Generate the markdown table
	var tableBuffer bytes.Buffer
	tableWriter := bufio.NewWriter(&tableBuffer)
	printMarkDownTable(tableWriter, header, rows)
	if err := tableWriter.Flush(); err != nil {
		return "", describeErr(f.GetModuleRoot(), "Flushing the table buffer failed", err)
	}
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
	keepOutputFile := false
	defer func() {
		if !keepOutputFile {
			_ = os.Remove(tmpFileName)
		}
	}()
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

			matched := markdownTableRowRegex.MatchString(line)

			if configSection && !foundTable && matched {
				foundTable = true
				if _, err := outputWriter.WriteString(tableBuffer.String() + "\n"); err != nil {
					return "", describeErr(f.GetModuleRoot(), "Writing the output file failed", err)
				}
			} else if configSection && foundTable && matched {
			} else if configSection && foundTable && !matched {
				configSection = false
			} else {
				if _, err := outputWriter.WriteString(line + "\n"); err != nil {
					return "", describeErr(f.GetModuleRoot(), "Writing the output file failed", err)
				}
			}
		} else {
			if _, err := outputWriter.WriteString(line + "\n"); err != nil {
				return "", describeErr(f.GetModuleRoot(), "Writing the output file failed", err)
			}
		}
	}
	if err := inputScanner.Err(); err != nil {
		return "", describeErr(f.GetModuleRoot(), "Reading the output file failed", err)
	}

	// If no table was found, append it to the end of the file
	if !foundTable {
		if _, err := outputWriter.WriteString("\n" + tableBuffer.String()); err != nil {
			return "", describeErr(f.GetModuleRoot(), "Writing the output file failed", err)
		}
	}

	err = outputWriter.Flush()
	if err != nil {
		return "", describeErr(f.GetModuleRoot(), "Failed to Flush Writer", err)
	}

	keepOutputFile = true
	return tmpFileName, nil
}

// configTableRows formats the config fields as Markdown table rows,
// marking optional keys with ? and required keys with ! and skipping
// the fields commented with +nodoc.
func configTableRows(fields []engine.ConfigField) [][]string {
	rows := make([][]string, 0, len(fields))
	for _, f := range fields {
		if f.NoDoc {
			continue
		}
		key := f.Key()
		switch {
		case f.Optional:
			key = strings.TrimSuffix(key, ":") + "?:"
		case f.Required:
			key = strings.TrimSuffix(key, ":") + "!:"
		}
		def := ""
		if f.Default != "" {
			def = fmt.Sprintf("`%s`", mdEscape(f.Default))
		}
		rows = append(rows, []string{
			fmt.Sprintf("`%s`", key),
			fmt.Sprintf("`%s`", mdEscape(f.Type)),
			def,
			strings.Join(strings.Fields(f.Doc), " "),
		})
	}
	return rows
}

// mdEscape escapes the pipe character so that code spans do not break the table.
func mdEscape(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

func printMarkDownTable(writer io.Writer, header []string, rows [][]string) {
	table := tablewriter.NewTable(writer,
		tablewriter.WithRenderer(renderer.NewBlueprint(tw.Rendition{
			Borders: tw.Border{Left: tw.On, Right: tw.On, Top: tw.Off, Bottom: tw.Off},
			Symbols: tw.NewSymbolCustom("markdown").
				WithColumn("|").
				WithRow("-").
				WithCenter("|").
				WithMidLeft("|").
				WithMidRight("|"),
			Settings: tw.Settings{
				Separators: tw.Separators{BetweenColumns: tw.On, BetweenRows: tw.Off},
				Lines:      tw.Lines{ShowHeaderLine: tw.On},
			},
		})),
		tablewriter.WithHeaderAutoFormat(tw.On),
		tablewriter.WithHeaderAutoWrap(tw.WrapNone),
		tablewriter.WithRowAutoWrap(tw.WrapNone),
		tablewriter.WithHeaderAlignment(tw.AlignLeft),
		tablewriter.WithRowAlignment(tw.AlignLeft),
		tablewriter.WithTrimSpace(tw.Off),
	)
	table.Header(header)
	_ = table.Bulk(rows)
	_ = table.Render()
}

func isMarkdownFile(filename string) bool {
	extension := strings.ToLower(filepath.Ext(filename))
	return extension == ".md" || extension == ".markdown"
}
