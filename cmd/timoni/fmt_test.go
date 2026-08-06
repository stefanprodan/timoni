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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

const (
	fmtTestUnformatted = "hello:   \"world\"\n"
	fmtTestFormatted   = "hello: \"world\"\n"
)

// writeFmtFixture writes content at path, creating parent directories.
func writeFmtFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// writeFmtModule writes a cue.mod/module.cue declaring the given
// language version under dir.
func writeFmtModule(t *testing.T, dir, version string) {
	t.Helper()
	writeFmtFixture(t, filepath.Join(dir, "cue.mod", "module.cue"),
		fmt.Sprintf("module: \"example.com/test\"\nlanguage: version: %q\n", version))
}

func readFmtFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestFmtModuleDir(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	writeFmtModule(t, dir, "v0.17.1")
	writeFmtFixture(t, filepath.Join(dir, "values.cue"), fmtTestUnformatted)
	writeFmtFixture(t, filepath.Join(dir, "templates", "config.cue"), fmtTestUnformatted)
	writeFmtFixture(t, filepath.Join(dir, "cue.mod", "gen", "vendored.cue"), fmtTestUnformatted)
	writeFmtFixture(t, filepath.Join(dir, "_tools", "skip.cue"), fmtTestUnformatted)
	writeFmtFixture(t, filepath.Join(dir, ".hidden", "skip.cue"), fmtTestUnformatted)

	_, err := executeCommand("fmt " + dir)
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(readFmtFile(t, filepath.Join(dir, "values.cue"))).To(Equal(fmtTestFormatted))
	g.Expect(readFmtFile(t, filepath.Join(dir, "templates", "config.cue"))).To(Equal(fmtTestFormatted))
	g.Expect(readFmtFile(t, filepath.Join(dir, "cue.mod", "gen", "vendored.cue"))).To(Equal(fmtTestUnformatted))
	g.Expect(readFmtFile(t, filepath.Join(dir, "_tools", "skip.cue"))).To(Equal(fmtTestUnformatted))
	g.Expect(readFmtFile(t, filepath.Join(dir, ".hidden", "skip.cue"))).To(Equal(fmtTestUnformatted))
}

func TestFmtSingleFile(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	bundle := filepath.Join(dir, "bundle.cue")
	writeFmtFixture(t, bundle, fmtTestUnformatted)

	_, err := executeCommand("fmt " + bundle)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(readFmtFile(t, bundle)).To(Equal(fmtTestFormatted))
}

func TestFmtExplicitSkippedDirs(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	genFile := filepath.Join(dir, "cue.mod", "gen", "vendored.cue")
	toolsDir := filepath.Join(dir, "_tools")
	writeFmtFixture(t, genFile, fmtTestUnformatted)
	writeFmtFixture(t, filepath.Join(toolsDir, "tool.cue"), fmtTestUnformatted)

	_, err := executeCommand("fmt " + genFile + " " + toolsDir)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(readFmtFile(t, genFile)).To(Equal(fmtTestFormatted))
	g.Expect(readFmtFile(t, filepath.Join(toolsDir, "tool.cue"))).To(Equal(fmtTestFormatted))
}

func TestFmtDiff(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	app := filepath.Join(dir, "app.cue")
	writeFmtFixture(t, app, fmtTestUnformatted)

	// Like flux diff: print the drift, then fail with a non-zero
	// exit; the closing error is logged by main like any other error.
	// The fixture lives outside the working directory, so the diff
	// header shows its full path.
	output, err := executeCommand("fmt --diff " + dir)
	g.Expect(err).To(MatchError(ContainSubstring("formatting differences found")))
	g.Expect(output).To(ContainSubstring(app))
	g.Expect(output).To(ContainSubstring("-" + fmtTestUnformatted))
	g.Expect(output).To(ContainSubstring("+" + fmtTestFormatted))
	g.Expect(readFmtFile(t, app)).To(Equal(fmtTestUnformatted))

	writeFmtFixture(t, app, fmtTestFormatted)
	output, err = executeCommand("fmt --diff " + dir)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(output).To(BeEmpty())
}

func TestFmtStdin(t *testing.T) {
	g := NewWithT(t)

	output, err := executeCommandWithIn("fmt -", strings.NewReader(fmtTestUnformatted))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(output).To(Equal(fmtTestFormatted))
}

func TestFmtStdinMixedWithPaths(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	app := filepath.Join(dir, "app.cue")
	writeFmtFixture(t, app, fmtTestUnformatted)

	output, err := executeCommandWithIn("fmt - "+app, strings.NewReader(fmtTestUnformatted))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(output).To(Equal(fmtTestFormatted))
	g.Expect(readFmtFile(t, app)).To(Equal(fmtTestFormatted))
}

func TestFmtIdempotent(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	writeFmtFixture(t, filepath.Join(dir, "app.cue"), fmtTestUnformatted)

	_, err := executeCommand("fmt " + dir)
	g.Expect(err).ToNot(HaveOccurred())

	_, err = executeCommand("fmt --diff " + dir)
	g.Expect(err).ToNot(HaveOccurred())
}

func TestFmtMalformed(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	app := filepath.Join(dir, "bad.cue")
	other := filepath.Join(dir, "good.cue")
	writeFmtFixture(t, app, "a: {\n")
	writeFmtFixture(t, other, fmtTestUnformatted)

	output, err := executeCommand("fmt " + dir)
	g.Expect(err).To(MatchError(ContainSubstring("formatting failed for 1 file(s)")))
	// The logged error must carry the file position, e.g. "bad.cue:1:6".
	g.Expect(output).To(MatchRegexp(`bad\.cue:\d+:\d+`))
	g.Expect(readFmtFile(t, app)).To(Equal("a: {\n"))
	// The run continues past the malformed file, gofmt-style.
	g.Expect(readFmtFile(t, other)).To(Equal(fmtTestFormatted))
}

func TestFmtRejectsNonCUEFile(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	readme := filepath.Join(dir, "README.md")
	writeFmtFixture(t, readme, "# readme\n")

	_, err := executeCommand("fmt " + readme)
	g.Expect(err).To(MatchError(ContainSubstring("not a CUE file")))
}

func TestFmtDotsAlias(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	writeFmtFixture(t, filepath.Join(dir, "app.cue"), fmtTestUnformatted)

	_, err := executeCommand("fmt " + dir + "/...")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(readFmtFile(t, filepath.Join(dir, "app.cue"))).To(Equal(fmtTestFormatted))
}

func TestFmtDotsInvalid(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	app := filepath.Join(dir, "app.cue")
	writeFmtFixture(t, app, fmtTestUnformatted)

	for _, arg := range []string{app + "/...", dir + "/...suffix", dir + "/pre...", dir + "/a...b"} {
		_, err := executeCommand("fmt " + arg)
		g.Expect(err).To(HaveOccurred(), "expected error for arg %s", arg)
	}
	g.Expect(readFmtFile(t, app)).To(Equal(fmtTestUnformatted))
}

func TestFmtSymlinks(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	outside := filepath.Join(dir, "outside")
	tree := filepath.Join(dir, "tree")
	writeFmtFixture(t, filepath.Join(outside, "target.cue"), fmtTestUnformatted)
	writeFmtFixture(t, filepath.Join(tree, "app.cue"), fmtTestUnformatted)
	g.Expect(os.Symlink(filepath.Join(outside, "target.cue"), filepath.Join(tree, "link.cue"))).To(Succeed())
	g.Expect(os.Symlink(outside, filepath.Join(dir, "treelink"))).To(Succeed())

	// Walks skip symlinked files, so the target stays untouched.
	_, err := executeCommand("fmt " + tree)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(readFmtFile(t, filepath.Join(tree, "app.cue"))).To(Equal(fmtTestFormatted))
	g.Expect(readFmtFile(t, filepath.Join(outside, "target.cue"))).To(Equal(fmtTestUnformatted))

	// An explicit symlinked directory argument is walked.
	_, err = executeCommand("fmt " + filepath.Join(dir, "treelink"))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(readFmtFile(t, filepath.Join(outside, "target.cue"))).To(Equal(fmtTestFormatted))
}

func TestFmtExplicitSymlinkedFile(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()

	// The target lives in a v0.17 module and uses newline-separated
	// list elements, which do not parse under v0.9 rules. The link
	// lives in a v0.9 module: formatting through it must use the
	// target module's version.
	target := filepath.Join(dir, "target")
	writeFmtModule(t, target, "v0.17.1")
	writeFmtFixture(t, filepath.Join(target, "versioned.cue"), "x: [\n1\n2\n]\n")
	linksite := filepath.Join(dir, "linksite")
	writeFmtModule(t, linksite, "v0.9.0")
	g.Expect(os.Symlink(filepath.Join(target, "versioned.cue"), filepath.Join(linksite, "versioned.cue"))).To(Succeed())

	_, err := executeCommand("fmt " + filepath.Join(linksite, "versioned.cue"))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(readFmtFile(t, filepath.Join(target, "versioned.cue"))).To(Equal("x: [\n\t1\n\t2\n]\n"))
}

func TestFmtVersionWiring(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()

	// Newline-separated list elements parse only under v0.17+ rules,
	// proving the resolved module version reaches the parser.
	oldMod := filepath.Join(dir, "old")
	writeFmtModule(t, oldMod, "v0.9.0")
	writeFmtFixture(t, filepath.Join(oldMod, "app.cue"), "x: [\n1\n2\n]\n")
	newMod := filepath.Join(dir, "new")
	writeFmtModule(t, newMod, "v0.17.1")
	writeFmtFixture(t, filepath.Join(newMod, "app.cue"), "x: [\n1\n2\n]\n")

	_, err := executeCommand("fmt " + oldMod)
	g.Expect(err).To(HaveOccurred())
	g.Expect(readFmtFile(t, filepath.Join(oldMod, "app.cue"))).To(Equal("x: [\n1\n2\n]\n"))

	_, err = executeCommand("fmt " + newMod)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(readFmtFile(t, filepath.Join(newMod, "app.cue"))).To(Equal("x: [\n\t1\n\t2\n]\n"))
}

func TestFmtResolveArgAlias(t *testing.T) {
	g := NewWithT(t)

	for _, tt := range []struct {
		arg  string
		want string
	}{
		{"...", "."},
		{"./...", "."},
		{"/...", "/"},
	} {
		got, err := resolveFmtArg(tt.arg)
		g.Expect(err).ToNot(HaveOccurred(), "arg %s", tt.arg)
		g.Expect(got).To(Equal(tt.want), "arg %s", tt.arg)
	}
}

func TestFmtVersionResolution(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()

	outer := filepath.Join(dir, "outer")
	writeFmtModule(t, outer, "v0.17.1")
	nested := filepath.Join(outer, "nested")
	writeFmtModule(t, nested, "v0.9.0")
	noVersion := filepath.Join(dir, "noversion")
	writeFmtFixture(t, filepath.Join(noVersion, "cue.mod", "module.cue"),
		"module: \"example.com/noversion\"\n")
	broken := filepath.Join(dir, "broken")
	writeFmtFixture(t, filepath.Join(broken, "cue.mod", "module.cue"), "not valid {{{\n")
	empty := filepath.Join(dir, "empty", "cue.mod")
	g.Expect(os.MkdirAll(empty, 0755)).To(Succeed())

	runner := &fmtRunner{versions: map[string]string{}}
	g.Expect(runner.versionFor(outer)).To(Equal("v0.17.1"))
	g.Expect(runner.versionFor(filepath.Join(outer, "templates"))).To(Equal("v0.17.1"))
	// The nearest cue.mod wins; a nested module never inherits the parent's version.
	g.Expect(runner.versionFor(nested)).To(Equal("v0.9.0"))
	g.Expect(runner.versionFor(filepath.Join(nested, "templates"))).To(Equal("v0.9.0"))
	// Fallbacks: no language.version, unparseable module.cue,
	// cue.mod without module.cue, and no module at all.
	g.Expect(runner.versionFor(noVersion)).To(BeEmpty())
	g.Expect(runner.versionFor(broken)).To(BeEmpty())
	g.Expect(runner.versionFor(filepath.Join(dir, "empty"))).To(BeEmpty())
	g.Expect(runner.versionFor(dir)).To(BeEmpty())
}

func TestFmtNestedModules(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	writeFmtModule(t, dir, "v0.17.1")
	writeFmtFixture(t, filepath.Join(dir, "app.cue"), fmtTestUnformatted)
	nested := filepath.Join(dir, "nested")
	writeFmtModule(t, nested, "v0.9.0")
	writeFmtFixture(t, filepath.Join(nested, "app.cue"), fmtTestUnformatted)

	_, err := executeCommand("fmt " + dir)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(readFmtFile(t, filepath.Join(dir, "app.cue"))).To(Equal(fmtTestFormatted))
	g.Expect(readFmtFile(t, filepath.Join(nested, "app.cue"))).To(Equal(fmtTestFormatted))
}
