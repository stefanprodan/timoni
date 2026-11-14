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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stefanprodan/timoni/internal/runtime"
)

var rollbackCmd = &cobra.Command{
	Use:   "rollback INSTANCE_NAME",
	Args:  cobra.MaximumNArgs(1),
	Short: "Rollback an instance to the previous module revision",
	Long: `The rollback command restores the previous module revision of an instance.

Timoni keeps one previous record (module URL, version, digest and values) in the
instance storage Secret. Rollback re-applies that revision with the stored values.
A second consecutive rollback is rejected until another successful apply records
a new previous revision. Timoni does not run down-migrations.
`,
	Example: `  # Rollback an instance to the previous revision
  timoni -n apps rollback app
`,
	RunE: runRollbackCmd,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		switch len(args) {
		case 0:
			return completeInstanceList(cmd, args, toComplete)
		default:
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
	},
}

type rollbackFlags struct {
	name  string
	wait  bool
	force bool
}

var rollbackArgs rollbackFlags

func init() {
	rollbackCmd.Flags().BoolVar(&rollbackArgs.wait, "wait", true,
		"Wait for the applied Kubernetes objects to become ready.")
	rollbackCmd.Flags().BoolVar(&rollbackArgs.force, "force", false,
		"Recreate immutable Kubernetes resources.")
	rootCmd.AddCommand(rollbackCmd)
}

func runRollbackCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("instance name is required")
	}
	rollbackArgs.name = args[0]

	log := loggerInstance(cmd.Context(), rollbackArgs.name)

	rm, err := runtime.NewResourceManager(kubeconfigArgs)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), rootArgs.timeout)
	defer cancel()

	st := runtime.NewStorageManager(rm)
	prev, err := st.GetPrevious(ctx, rollbackArgs.name, *kubeconfigArgs.Namespace)
	if err != nil {
		return err
	}
	if prev == nil {
		return fmt.Errorf("no previous revision found for instance %q", rollbackArgs.name)
	}
	if prev.Module.Repository == "" {
		return fmt.Errorf("previous revision for instance %q is missing module repository", rollbackArgs.name)
	}

	tmpDir, err := os.MkdirTemp("", "timoni-rollback")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	valuesFile := filepath.Join(tmpDir, "values.cue")
	if err := os.WriteFile(valuesFile, fmt.Appendf(nil, "values: %s\n", prev.Values), 0o600); err != nil {
		return err
	}

	applyArgs = applyFlags{
		name:        rollbackArgs.name,
		module:      prev.Module.Repository,
		valuesFiles: []string{valuesFile},
		wait:        rollbackArgs.wait,
		force:       rollbackArgs.force,
	}

	// Prefer @digest pull for OCI artifacts. Local modules store digest
	// "unknown"; fall back to the recorded version in that case.
	digest := prev.Module.Digest
	if digest != "" && digest != "unknown" && strings.Contains(digest, ":") {
		log.Info(fmt.Sprintf("rolling back to %s@%s", prev.Module.Repository, digest))
		if err := applyArgs.digest.Set(digest); err != nil {
			return err
		}
	} else if prev.Module.Version != "" {
		log.Info(fmt.Sprintf("rolling back to %s:%s", prev.Module.Repository, prev.Module.Version))
		if err := applyArgs.version.Set(prev.Module.Version); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("previous revision for instance %q is missing module digest and version", rollbackArgs.name)
	}

	applyFromRollback = true
	return runApplyCmd(cmd, []string{rollbackArgs.name, prev.Module.Repository})
}
