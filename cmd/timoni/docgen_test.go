/*
Copyright 2026 Stefan Prodan

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
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
)

func TestDocgen(t *testing.T) {
	g := NewWithT(t)

	root := &cobra.Command{Use: "tool", Short: "A tool."}
	root.DisableAutoGenTag = true
	child := &cobra.Command{
		Use:   "run <name>",
		Short: "Run the thing.",
		Long: `Run the thing named <name>.
Values are set with 'key=value' pairs and stored in tool.<name> as {json}.
Inline code like ` + "`<kept>`" + ` stays as is.
With --dry-run=false the thing runs.`,
		Example: `  # Run with a placeholder
  tool run <name> --set key={value}`,
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
	child.Flags().String("set", "", "Set a <key>=<value> pair.")
	root.AddCommand(child)

	dir := t.TempDir()
	g.Expect(genMDXTree(root, dir)).To(Succeed())

	entries, err := os.ReadDir(dir)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(entries).To(HaveLen(2))

	page, err := os.ReadFile(filepath.Join(dir, "tool_run.mdx"))
	g.Expect(err).ToNot(HaveOccurred())
	out := string(page)

	g.Expect(out).To(HavePrefix("---\ntitle: \"tool run\"\ndescription: \"Run the thing.\"\nmode: \"wide\"\n---\n\n"))
	g.Expect(out).ToNot(ContainSubstring("## tool run"))
	g.Expect(out).ToNot(ContainSubstring("\nRun the thing.\n"))
	g.Expect(out).To(ContainSubstring("\n## Synopsis\n"))
	g.Expect(out).To(ContainSubstring("\n## Options\n"))
	g.Expect(out).ToNot(ContainSubstring("\n### "))
	g.Expect(out).To(ContainSubstring("Run the thing named &lt;name&gt;."))
	g.Expect(out).To(ContainSubstring("Values are set with `key=value` pairs and stored in tool.&lt;name&gt; as \\{json\\}."))
	g.Expect(out).To(ContainSubstring("Inline code like `<kept>` stays as is."))
	g.Expect(out).To(ContainSubstring("With `--dry-run=false` the thing runs."))
	g.Expect(out).To(ContainSubstring("  tool run <name> --set key={value}\n"))
	g.Expect(out).To(ContainSubstring("\n## Examples\n\n```sh\n"))
	g.Expect(out).To(ContainSubstring("--set string   Set a <key>=<value> pair."))
	g.Expect(out).To(ContainSubstring("\n## See also\n"))
	g.Expect(out).To(ContainSubstring("* [tool](/cmd/tool)"))

	rootPage, err := os.ReadFile(filepath.Join(dir, "tool.mdx"))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(rootPage)).To(ContainSubstring("* [tool run](/cmd/tool_run)"))
}
