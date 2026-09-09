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
	"context"
	"fmt"
	"path/filepath"
	"slices"

	"cuelang.org/go/cue/cuecontext"
	"github.com/spf13/cobra"

	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/oci"
)

type bundleUpdateTransaction struct {
	plan         *engine.UpdatePlan
	changedFiles []string
	originals    map[string][]byte
	updated      map[string][]byte
	overrides    map[string][]byte
}

func prepareBundleUpdate(cmd *cobra.Command, files []string, workdir, level, localIndex string, localOCI []string, creds string) (*bundleUpdateTransaction, error) {
	ctx, cancel := context.WithTimeout(cmd.Context(), rootArgs.timeout)
	defer cancel()

	updater := engine.NewBundleUpdater(cuecontext.New(), files)
	updater.SetWorkdir(workdir)
	if err := updater.SetLevel(level); err != nil {
		return nil, err
	}

	lister := engine.ModuleVersionLister(&engine.OCIModuleVersionLister{
		Opts: oci.Options(ctx, creds, rootArgs.registryInsecure),
	})
	var err error
	if localIndex != "" {
		localLister, err := engine.NewLocalModuleIndexLister(localIndex, lister)
		if err != nil {
			return nil, err
		}
		updater.SetLocalIndex(localLister)
		lister = localLister
	}
	if len(localOCI) > 0 {
		artifactLister, err := engine.NewLocalModuleArtifactLister(localOCI, lister)
		if err != nil {
			return nil, err
		}
		updater.SetLocalArtifacts(artifactLister)
		lister = artifactLister
	}

	if err := updater.Load(); err != nil {
		return nil, describeErr(workdir, "failed to build bundle", err)
	}

	plan, err := updater.Plan(ctx, lister)
	if plan != nil {
		log := LoggerFrom(cmd.Context())
		for _, skip := range plan.Skipped {
			log.Info(fmt.Sprintf("instance %s skipped: %s", skip.Instance, skip.Reason))
		}
	}
	if err != nil {
		return nil, err
	}

	tx := &bundleUpdateTransaction{
		plan:      plan,
		originals: make(map[string][]byte),
		updated:   make(map[string][]byte),
		overrides: make(map[string][]byte),
	}
	for _, change := range plan.Changes {
		for _, file := range change.Files {
			if !slices.Contains(tx.changedFiles, file) {
				tx.changedFiles = append(tx.changedFiles, file)
			}
		}
	}
	slices.Sort(tx.changedFiles)

	for _, file := range tx.changedFiles {
		tx.originals[file], err = updater.Source(file)
		if err != nil {
			return nil, err
		}
	}
	if len(tx.changedFiles) == 0 {
		return tx, nil
	}

	if err := updater.Apply(plan); err != nil {
		return nil, describeErr(workdir, "update failed", err)
	}
	for _, file := range tx.changedFiles {
		tx.updated[file], err = updater.Format(file)
		if err != nil {
			return nil, err
		}
	}
	tx.changedFiles = slices.DeleteFunc(tx.changedFiles, func(file string) bool {
		return bytes.Equal(tx.originals[file], tx.updated[file])
	})
	for _, file := range tx.changedFiles {
		tx.overrides[file] = tx.updated[file]
		if abs, err := filepath.Abs(file); err == nil {
			tx.overrides[abs] = tx.updated[file]
		}
	}
	return tx, nil
}

func runBundleBuildWithUpdate(cmd *cobra.Command, files []string, workdir string) error {
	tx, err := prepareBundleUpdate(cmd, files, workdir, bundleBuildArgs.level, bundleBuildArgs.localIndex, bundleBuildArgs.localOCI, bundleBuildArgs.creds.String())
	if err != nil {
		return err
	}
	log := LoggerFrom(cmd.Context())
	for _, change := range tx.plan.Changes {
		log.Info(describeChange(change))
	}

	if err := runBundleUpdateVet(cmd, files, tx.overrides); err != nil {
		return err
	}

	var rendered bytes.Buffer
	out := cmd.OutOrStdout()
	cmd.SetOut(&rendered)
	err = runBundleBuild(cmd, files, workdir, tx.overrides)
	cmd.SetOut(nil)
	if err != nil {
		return err
	}

	if err := writeBundleFiles(tx.changedFiles, tx.originals, tx.updated); err != nil {
		return err
	}
	for _, file := range tx.changedFiles {
		log.Info(fmt.Sprintf("updated %s", fmtRelPath(file)))
	}
	_, err = out.Write(rendered.Bytes())
	return err
}
