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
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/flags"
	"github.com/stefanprodan/timoni/internal/logger"
)

var bundleUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update the module versions referenced in a bundle",
	Long: `The bundle update command lists the versions published for the modules
referenced in a bundle and rewrites the module version and digest fields
in the bundle CUE files according to the update policy declared on each
field with a '@timoni(update:...)' attribute. The fields are rewritten
in the files passed with '-f' and in the package files of the CUE module
that they import:

  @timoni(update:semver:<constraint>)  select the newest version matching the semver constraint
  @timoni(update:digest)               keep the version and refresh the digest
  @timoni(update:none)                 exclude the module reference from updates

References without an attribute follow the '--level' flag.

With '--local-index', an explicit CUE index maps module identities and semantic
versions to verified local sources. A matching OCI reference can switch to one
of those sources; unindexed OCI references continue to use the registry.

With '--oci', an explicit 'oci://repository=path' mapping adds a local OCI
archive or image layout for the specified repository. Timoni verifies the
manifest version and artifact digest; it never infers the repository from the
path.

With '--vet', Timoni validates the staged bundle before writing updates.
With '--dry-run --vet', it validates the staged update without writing files.
Use 'bundle build --update' when the updated modules must also be rendered.
`,
	Example: `  # Update the module references according to the policies declared in the bundle
  timoni bundle update -f bundle.cue

  # Update the references without a policy to the newest minor version
  timoni bundle update -f bundle.cue --level minor

  # Print the available updates without modifying the files
  timoni bundle update -f bundle.cue --dry-run

  # Update an indexed local module
  timoni bundle update --local-index module-index.cue -f bundle.cue

  # Update from a local OCI archive, explicitly mapped to its repository
  timoni bundle update --oci oci://registry.example/team/app=.local/artifacts/app.oci.tar -f bundle.cue

  # Validate the staged update before writing it
  timoni bundle update --vet -f bundle.cue
`,
	Args: cobra.NoArgs,
	RunE: runBundleUpdateCmd,
}

type bundleUpdateFlags struct {
	files      []string
	creds      flags.Credentials
	level      string
	dryrun     bool
	vet        bool
	localIndex string
	localOCI   []string
}

var bundleUpdateArgs bundleUpdateFlags

func init() {
	bundleUpdateCmd.Flags().StringSliceVarP(&bundleUpdateArgs.files, "file", "f", nil,
		"The local path to bundle.cue files.")
	bundleUpdateCmd.Flags().Var(&bundleUpdateArgs.creds, bundleUpdateArgs.creds.Type(), bundleUpdateArgs.creds.Description())
	bundleUpdateCmd.Flags().StringVar(&bundleUpdateArgs.level, "level", engine.UpdateLevelNone,
		"The update level for the module references without an update attribute, one of: none, patch, minor, major.")
	bundleUpdateCmd.Flags().BoolVar(&bundleUpdateArgs.dryrun, "dry-run", false,
		"Print the available updates without modifying the files.")
	bundleUpdateCmd.Flags().BoolVar(&bundleUpdateArgs.vet, "vet", false,
		"Validate the staged bundle before writing updates.")
	bundleUpdateCmd.Flags().StringVar(&bundleUpdateArgs.localIndex, "local-index", "",
		"CUE file that maps module identities and semantic versions to verified local sources.")
	bundleUpdateCmd.Flags().StringArrayVar(&bundleUpdateArgs.localOCI, "oci", nil,
		"Map a local OCI archive or image layout to a repository as 'oci://repository=path'; repeatable.")
	bundleCmd.AddCommand(bundleUpdateCmd)
}

func runBundleUpdateCmd(cmd *cobra.Command, args []string) error {
	log := LoggerFrom(cmd.Context())
	files := bundleUpdateArgs.files
	if len(files) == 0 {
		return fmt.Errorf("no bundle provided with -f")
	}

	workdir, err := resolveWorkdir(bundleArgs.workdir)
	if err != nil {
		return err
	}

	tx, err := prepareBundleUpdate(cmd, files, workdir, bundleUpdateArgs.level, bundleUpdateArgs.localIndex, bundleUpdateArgs.localOCI, bundleUpdateArgs.creds.String())
	if err != nil {
		return err
	}

	if bundleUpdateArgs.vet {
		if err := runBundleUpdateVet(cmd, files, tx.overrides); err != nil {
			return err
		}
	}

	if len(tx.plan.Changes) == 0 {
		log.Info("all module references are up to date")
		return nil
	}

	for _, change := range tx.plan.Changes {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), describeChange(change)); err != nil {
			return err
		}
	}

	if bundleUpdateArgs.dryrun {
		log.Info(fmt.Sprintf("%d module reference(s) can be updated in %s %s",
			len(tx.plan.Changes), strings.Join(relPaths(tx.changedFiles), ", "), logger.ColorizeDryRun("(dry run)")))
		return nil
	}

	if err := writeBundleFiles(tx.changedFiles, tx.originals, tx.updated); err != nil {
		return err
	}
	for _, file := range tx.changedFiles {
		log.Info(fmt.Sprintf("updated %s", fmtRelPath(file)))
	}
	return nil
}

// runBundleUpdateVet validates a bundle with the in-memory overrides used by
// the atomic 'bundle build --update' path.
func runBundleUpdateVet(cmd *cobra.Command, files []string, overrides map[string][]byte) error {
	offline := bundleArgs.runtimeFromEnv || len(bundleArgs.runtimeFiles) == 0
	return runBundleVet(cmd, files, offline, overrides)
}

// writeBundleFiles writes the updated content of the given files after
// verifying that none of them changed on disk since the bundle was loaded.
// When a write fails, the files already written are restored to their
// original content.
func writeBundleFiles(files []string, originals, updated map[string][]byte) error {
	for _, file := range files {
		current, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		if !bytes.Equal(current, originals[file]) {
			return fmt.Errorf("%s changed on disk since it was loaded, run the update again", fmtRelPath(file))
		}
	}

	for i, file := range files {
		if err := os.WriteFile(file, updated[file], 0o644); err != nil {
			var errs []error
			for _, written := range files[:i] {
				if rerr := os.WriteFile(written, originals[written], 0o644); rerr != nil {
					errs = append(errs, fmt.Errorf("failed to restore %s: %w", fmtRelPath(written), rerr))
				}
			}
			return errors.Join(append([]error{err}, errs...)...)
		}
	}
	return nil
}

// relPaths returns the given paths relative to the working directory.
func relPaths(paths []string) []string {
	rel := make([]string, 0, len(paths))
	for _, p := range paths {
		rel = append(rel, fmtRelPath(p))
	}
	return rel
}

// describeChange formats a module reference change as a single line.
func describeChange(change *engine.UpdateChange) string {
	from := change.FromVersion
	to := change.ToVersion
	if change.ToDigest != "" {
		if change.FromDigest != "" {
			from += "@" + change.FromDigest
		}
		to += "@" + change.ToDigest
	}
	source := ""
	if change.ToSource != "" {
		source = fmt.Sprintf(" source %s -> %s", change.FromSource, change.ToSource)
	}
	return fmt.Sprintf("%s: %s%s %s -> %s", strings.Join(change.Instances, ", "), change.Repository, source, from, to)
}
