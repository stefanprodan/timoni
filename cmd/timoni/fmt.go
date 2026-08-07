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
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	cueerrors "cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/mod/modfile"
	"github.com/go-logr/logr"
	"github.com/rogpeppe/go-internal/diff"
	"github.com/spf13/cobra"
)

var fmtCmd = &cobra.Command{
	Use:   "fmt [PATH ...]",
	Short: "Format CUE files",
	Long: `The fmt command rewrites CUE files in the standard format.

Arguments are files or directories, defaulting to the current directory.
Directories are formatted recursively; during the walk, directories named
"cue.mod" and those with a "." or "_" prefix are skipped unless given as
explicit arguments.

Files inside a module are parsed under the language version declared in
the module's cue.mod/module.cue, matching the behavior of Timoni's module
builds; files outside any module use the CUE version built into Timoni.

When run with --diff, no files are rewritten, the pending changes are
printed as unified diffs and the command fails if any file is not well
formatted.`,
	Example: `  # Format the current directory recursively
  timoni fmt

  # Verify formatting and print a diff for each file that needs changes
  timoni fmt --diff

  # Show the changes formatting would make to a bundle
  timoni fmt --diff bundle.cue

  # Format standard input, for editor integration
  timoni fmt -
`,
	RunE: runFmtCmd,
}

type fmtFlags struct {
	diff bool
}

var fmtArgs fmtFlags

func init() {
	fmtCmd.Flags().BoolVarP(&fmtArgs.diff, "diff", "d", false,
		"Print the diffs and exits 1 if any file needs formatting.")
	rootCmd.AddCommand(fmtCmd)
}

type fmtRunner struct {
	cmd         *cobra.Command
	log         logr.Logger
	versions    map[string]string
	unformatted bool
	failed      int
}

func runFmtCmd(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		args = []string{"."}
	}

	runner := &fmtRunner{
		cmd:      cmd,
		log:      LoggerFrom(cmd.Context()),
		versions: map[string]string{},
	}
	for _, arg := range args {
		if arg == "-" {
			if err := runner.formatStdin(); err != nil {
				runner.reportError(err)
			}
			continue
		}

		target, err := resolveFmtArg(arg)
		if err != nil {
			return err
		}

		info, err := os.Stat(target)
		if err != nil {
			return err
		}

		if info.IsDir() {
			if err := runner.walkDir(target); err != nil {
				return err
			}
			continue
		}

		if !strings.HasSuffix(target, ".cue") {
			return fmt.Errorf("not a CUE file: %s", arg)
		}
		// Resolve explicit symlinked files so the language version
		// comes from the target's module, not the link's location.
		resolved, err := filepath.EvalSymlinks(target)
		if err != nil {
			return err
		}
		if err := runner.formatFile(resolved); err != nil {
			runner.reportError(err)
		}
	}

	if runner.failed > 0 {
		return fmt.Errorf("formatting failed for %d file(s)", runner.failed)
	}
	if fmtArgs.diff && runner.unformatted {
		return fmt.Errorf("formatting differences found")
	}
	return nil
}

// reportError logs a per-file failure and keeps the run going,
// gofmt-style, so one malformed file does not block the rest of the
// tree. The run still exits non-zero at the end.
func (r *fmtRunner) reportError(err error) {
	r.failed++
	r.log.Error(nil, err.Error())
}

// resolveFmtArg cleans an argument path, accepting a trailing "/..."
// (or a bare "...") as an alias for the directory it follows.
func resolveFmtArg(arg string) (string, error) {
	if !strings.Contains(arg, "...") {
		return filepath.Clean(arg), nil
	}

	trimmed, found := strings.CutSuffix(arg, "...")
	if !found || strings.Contains(trimmed, "...") {
		return "", fmt.Errorf("invalid path %q: %q is only accepted as a trailing directory alias", arg, "...")
	}
	switch {
	case trimmed == "":
		trimmed = "."
	case strings.HasSuffix(trimmed, "/") || strings.HasSuffix(trimmed, string(filepath.Separator)):
		trimmed = filepath.Clean(trimmed)
	default:
		return "", fmt.Errorf("invalid path %q: %q must follow a directory separator", arg, "...")
	}

	info, err := os.Stat(trimmed)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("invalid path %q: %s is not a directory", arg, trimmed)
	}
	return trimmed, nil
}

// walkDir formats every regular CUE file under root, skipping cue.mod,
// dot and underscore directories, and symlinks. A symlinked root is
// resolved first so that explicitly named symlinked directories are
// walked instead of silently ignored.
func (r *fmtRunner) walkDir(root string) error {
	if li, err := os.Lstat(root); err == nil && li.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(root)
		if err != nil {
			return err
		}
		root = resolved
	}

	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			name := d.Name()
			isMod := name == "cue.mod"
			isDot := strings.HasPrefix(name, ".") && name != "." && name != ".."
			if path != root && (isMod || isDot || strings.HasPrefix(name, "_")) {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(path, ".cue") || !d.Type().IsRegular() {
			return nil
		}
		if err := r.formatFile(path); err != nil {
			r.reportError(err)
		}
		return nil
	})
}

// formatFile formats a single CUE file in place. With --diff the
// changes are displayed instead of written.
func (r *fmtRunner) formatFile(filename string) error {
	src, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	formatted, err := formatCUE(filename, src, r.versionFor(filepath.Dir(filename)))
	if err != nil {
		return err
	}
	if bytes.Equal(formatted, src) {
		return nil
	}
	r.unformatted = true

	if fmtArgs.diff {
		path := fmtRelPath(filename)
		_, err = fmt.Fprintln(r.cmd.OutOrStdout(), string(diff.Diff(path+".orig", src, path, formatted)))
		return err
	}
	return os.WriteFile(filename, formatted, 0644)
}

// formatStdin formats CUE read from standard input and writes the
// result to standard output. With --diff the changes are displayed
// instead.
func (r *fmtRunner) formatStdin() error {
	src, err := io.ReadAll(r.cmd.InOrStdin())
	if err != nil {
		return err
	}

	formatted, err := formatCUE("-", src, "")
	if err != nil {
		return err
	}

	if !fmtArgs.diff {
		_, err = r.cmd.OutOrStdout().Write(formatted)
		return err
	}
	if bytes.Equal(formatted, src) {
		return nil
	}
	r.unformatted = true

	_, err = fmt.Fprintln(r.cmd.OutOrStdout(), string(diff.Diff("-.orig", src, "-", formatted)))
	return err
}

// formatCUE returns the given CUE source in the standard format,
// parsing it under langVersion when one is declared. Parse errors are
// rendered with their source positions.
func formatCUE(filename string, src []byte, langVersion string) ([]byte, error) {
	opts := []parser.Option{parser.ParseComments}
	if langVersion != "" {
		opts = append(opts, parser.Version(langVersion))
	}
	syntax, err := parser.ParseFile(filename, src, opts...)
	if err != nil {
		cwd, _ := os.Getwd()
		return nil, fmt.Errorf("%s", strings.TrimSpace(cueerrors.Details(err, &cueerrors.Config{Cwd: cwd})))
	}
	return format.Node(syntax)
}

// versionFor returns the language version declared by the module
// enclosing dir, or an empty string when dir is outside any module or
// the version cannot be determined, in which case parsing falls back
// to the CUE version built into timoni.
func (r *fmtRunner) versionFor(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	return r.lookupVersion(abs)
}

// lookupVersion resolves the language version for an absolute
// directory by finding the nearest ancestor containing cue.mod. The
// search stops at the first cue.mod so a nested module never inherits
// a parent module's version. Results are memoized per directory.
func (r *fmtRunner) lookupVersion(dir string) string {
	if v, ok := r.versions[dir]; ok {
		return v
	}

	var v string
	if _, err := os.Stat(filepath.Join(dir, "cue.mod")); err == nil {
		v = readLanguageVersion(filepath.Join(dir, "cue.mod", "module.cue"))
	} else if parent := filepath.Dir(dir); parent != dir {
		v = r.lookupVersion(parent)
	}
	r.versions[dir] = v
	return v
}

// readLanguageVersion returns the language.version declared in a
// cue.mod/module.cue file, or an empty string when the file is
// missing, unparseable, or declares no version.
func readLanguageVersion(modFilePath string) string {
	data, err := os.ReadFile(modFilePath)
	if err != nil {
		return ""
	}
	mf, err := modfile.ParseNonStrict(data, modFilePath)
	if err != nil || mf.Language == nil {
		return ""
	}
	return mf.Language.Version
}

// fmtRelPath returns filename relative to the working directory for
// display. Files outside the working directory keep the name as
// given, avoiding long "../" chains.
func fmtRelPath(filename string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return filename
	}
	rel, err := filepath.Rel(cwd, filename)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filename
	}
	return rel
}
