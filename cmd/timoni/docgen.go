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
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

var (
	cmdDocPath string
)

var docgenCmd = &cobra.Command{
	Use:    "docgen",
	Args:   cobra.NoArgs,
	Short:  "Generate the documentation for the CLI commands.",
	Hidden: true,
	RunE:   docgenCmdRun,
}

func init() {
	docgenCmd.Flags().StringVar(&cmdDocPath, "path", "./docs/cmd", "path to write the generated documentation to")

	rootCmd.AddCommand(docgenCmd)
}

func docgenCmdRun(cmd *cobra.Command, args []string) error {
	if err := os.MkdirAll(cmdDocPath, 0777); err != nil {
		return err
	}

	return genMDXTree(rootCmd, cmdDocPath)
}

// genMDXTree writes one MDX page per visible command, recursively.
func genMDXTree(cmd *cobra.Command, dir string) error {
	for _, c := range cmd.Commands() {
		if !c.IsAvailableCommand() || c.IsAdditionalHelpTopicCommand() {
			continue
		}
		if err := genMDXTree(c, dir); err != nil {
			return err
		}
	}

	basename := strings.ReplaceAll(cmd.CommandPath(), " ", "_")
	filename := filepath.Join(dir, basename+".mdx")

	page, err := genMDXPage(cmd)
	if err != nil {
		return err
	}
	return os.WriteFile(filename, page, 0644)
}

// genMDXPage renders a command as a Mintlify MDX page: YAML frontmatter with
// the title, description and wide mode (no table of contents), followed by the cobra Markdown reference with the
// title heading and short description removed (both live in the frontmatter),
// section headings promoted to H2, the examples fence tagged as shell, and MDX-sensitive characters escaped in prose.
func genMDXPage(cmd *cobra.Command) ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := doc.GenMarkdownCustom(cmd, buf, linkHandler); err != nil {
		return nil, err
	}

	body := stripTitleHeading(buf.String(), cmd.CommandPath())
	body = strings.TrimPrefix(body, strings.TrimSpace(cmd.Short)+"\n\n")
	body = strings.TrimPrefix(strings.ReplaceAll("\n"+body, "\n### ", "\n## "), "\n")
	body = strings.ReplaceAll(body, "## SEE ALSO\n", "## See also\n")
	body = strings.ReplaceAll(body, "## Examples\n\n```\n", "## Examples\n\n```sh\n")
	body = escapeMDX(body)

	page := fmt.Sprintf("---\ntitle: %q\ndescription: %q\nmode: \"wide\"\n---\n\n%s",
		cmd.CommandPath(), strings.TrimSpace(cmd.Short), body)
	return []byte(page), nil
}

func stripTitleHeading(md, title string) string {
	return strings.TrimPrefix(md, "## "+title+"\n\n")
}

// escapeMDX escapes characters that MDX would parse as JSX or expressions,
// leaving fenced code blocks and inline code untouched.
func escapeMDX(md string) string {
	var out strings.Builder
	inFence := false
	for _, line := range strings.SplitAfter(md, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			out.WriteString(line)
			continue
		}
		if inFence {
			out.WriteString(line)
			continue
		}
		out.WriteString(escapeMDXLine(line))
	}
	return out.String()
}

// quotedLiteral matches single-quoted tokens without whitespace, the cobra
// convention for flags and paths in help text, e.g. '--values' or '~/.docker'.
var quotedLiteral = regexp.MustCompile(`'(\S+?)'`)

// bareFlag matches unquoted long flags in prose, e.g. --wait=false.
var bareFlag = regexp.MustCompile(`(^|\s)(--[a-z][\w-]*(?:=\S+)?)`)

func escapeMDXLine(line string) string {
	if !strings.Contains(line, "`") {
		line = quotedLiteral.ReplaceAllString(line, "`$1`")
		line = bareFlag.ReplaceAllString(line, "$1`$2`")
	}

	var out strings.Builder
	inCode := false
	for _, r := range line {
		switch {
		case r == '`':
			inCode = !inCode
			out.WriteRune(r)
		case inCode:
			out.WriteRune(r)
		case r == '<':
			out.WriteString("&lt;")
		case r == '>':
			out.WriteString("&gt;")
		case r == '{':
			out.WriteString("\\{")
		case r == '}':
			out.WriteString("\\}")
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

func linkHandler(name string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	return "/cmd/" + strings.ToLower(base)
}
