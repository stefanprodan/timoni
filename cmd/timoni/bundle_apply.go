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
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/engine/fetcher"
	"github.com/stefanprodan/timoni/internal/flags"
	"github.com/stefanprodan/timoni/internal/logger"
	"github.com/stefanprodan/timoni/internal/reconciler"
	"github.com/stefanprodan/timoni/internal/runtime"
)

var bundleApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Install or upgrade instances from a bundle",
	Long: `The bundle apply command installs or upgrades the instances defined in a bundle.
`,
	Example: `  # Install all instances from a bundle
  timoni bundle apply -f bundle.cue

  # Do a dry-run upgrade and print the diff
  timoni bundle apply -f bundle.cue \
  --dry-run --diff

  # Force apply instances from multiple bundles
  timoni bundle apply --force \
  -f ./bundle.cue \
  -f ./bundle_secrets.cue

  # Pass secret values from stdin
  cat ./bundle_secrets.cue | timoni bundle apply -f ./bundle.cue -f -
`,
	Args: cobra.NoArgs,
	RunE: runBundleApplyCmd,
}

type bundleApplyFlags struct {
	pkg                flags.Package
	files              []string
	dryrun             bool
	diff               bool
	wait               bool
	force              bool
	overwriteOwnership bool
	creds              flags.Credentials
}

var bundleApplyArgs bundleApplyFlags

func init() {
	bundleApplyCmd.Flags().VarP(&bundleApplyArgs.pkg, bundleApplyArgs.pkg.Type(), bundleApplyArgs.pkg.Shorthand(), bundleApplyArgs.pkg.Description())
	bundleApplyCmd.Flags().StringSliceVarP(&bundleApplyArgs.files, "file", "f", nil,
		"The local path to bundle.cue files.")
	bundleApplyCmd.Flags().BoolVar(&bundleApplyArgs.force, "force", false,
		"Recreate immutable Kubernetes resources.")
	bundleApplyCmd.Flags().BoolVar(&bundleApplyArgs.overwriteOwnership, "overwrite-ownership", false,
		"Overwrite instance ownership, if any instances are owned by other Bundles.")
	bundleApplyCmd.Flags().BoolVar(&bundleApplyArgs.dryrun, "dry-run", false,
		"Perform a server-side apply dry run.")
	bundleApplyCmd.Flags().BoolVar(&bundleApplyArgs.diff, "diff", false,
		"Perform a server-side apply dry run and prints the diff.")
	bundleApplyCmd.Flags().BoolVar(&bundleApplyArgs.wait, "wait", true,
		"Wait for the applied Kubernetes objects to become ready.")
	bundleApplyCmd.Flags().Var(&bundleApplyArgs.creds, bundleApplyArgs.creds.Type(), bundleApplyArgs.creds.Description())
	bundleCmd.AddCommand(bundleApplyCmd)
}

func runBundleApplyCmd(cmd *cobra.Command, _ []string) error {
	start := time.Now()
	files := bundleApplyArgs.files
	if len(files) == 0 {
		return errors.New("no bundle provided with -f")
	}
	var stdinFile string
	for i, file := range files {
		if file == "-" {
			stdinFile, err := saveReaderToFile(cmd.InOrStdin())
			if err != nil {
				return err
			}
			files[i] = stdinFile
			break
		}
	}
	if stdinFile != "" {
		defer os.Remove(stdinFile)
	}

	tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	ctx, cancel := context.WithTimeout(cmd.Context(), rootArgs.timeout)
	defer cancel()

	cuectx := cuecontext.New()
	bm := engine.NewBundleBuilder(cuectx, files)

	runtimeValues := make(map[string]string)

	if bundleArgs.runtimeFromEnv {
		maps.Copy(runtimeValues, engine.GetEnv())
	}

	rt, err := buildRuntime(bundleArgs.runtimeFiles)
	if err != nil {
		return err
	}

	clusters := rt.SelectClusters(bundleArgs.runtimeCluster, bundleArgs.runtimeClusterGroup)
	if len(clusters) == 0 {
		return errors.New("no cluster found")
	}

	ctxPull, cancel := context.WithTimeout(ctx, rootArgs.timeout)
	defer cancel()

	for _, cluster := range clusters {
		kubeconfigArgs.Context = &cluster.KubeContext

		clusterValues := make(map[string]string)

		// add values from env
		maps.Copy(clusterValues, runtimeValues)

		// add values from cluster
		rm, err := runtime.NewResourceManager(kubeconfigArgs)
		if err != nil {
			return err
		}
		reader := runtime.NewResourceReader(rm)
		rv, err := reader.Read(ctx, rt.Refs)
		if err != nil {
			return err
		}
		maps.Copy(clusterValues, rv)

		// add cluster info
		maps.Copy(clusterValues, cluster.NameGroupValues())

		// create cluster workspace
		workspace := path.Join(tmpDir, cluster.Name)
		if err := os.MkdirAll(workspace, os.ModePerm); err != nil {
			return err
		}

		if err := bm.InitWorkspace(workspace, clusterValues); err != nil {
			return describeErr(workspace, "failed to parse bundle", err)
		}

		v, err := bm.Build(workspace)
		if err != nil {
			return describeErr(tmpDir, "failed to build bundle", err)
		}

		bundle, err := bm.GetBundle(v)
		if err != nil {
			return err
		}

		log := loggerBundle(cmd.Context(), bundle.Name, cluster.Name, true)

		if !bundleApplyArgs.overwriteOwnership {
			err = bundleInstancesOwnershipConflicts(bundle.Instances)
			if err != nil {
				return annotateInstanceOwnershipConflictErr(err)
			}
		}

		for _, instance := range bundle.Instances {
			spin := logger.StartSpinner(fmt.Sprintf("pulling %s", instance.Module.Repository))
			pullErr := fetchBundleInstanceModule(ctxPull, instance, tmpDir)
			spin.Stop()
			if pullErr != nil {
				return pullErr
			}
		}

		kubeVersion, err := runtime.ServerVersion(kubeconfigArgs)
		if err != nil {
			return err
		}

		startMsg := fmt.Sprintf("applying %v instance(s)", len(bundle.Instances))
		if !cluster.IsDefault() {
			startMsg = fmt.Sprintf("%s on %s", startMsg, logger.ColorizeSubject(cluster.Group))
		}

		if bundleApplyArgs.dryrun || bundleApplyArgs.diff {
			log.Info(fmt.Sprintf("%s %s", startMsg, logger.ColorizeDryRun("(server dry run)")))
		} else {
			log.Info(startMsg)
		}

		for _, instance := range bundle.Instances {
			instance.Cluster = cluster.Name
			if err := applyBundleInstance(logr.NewContext(ctx, log), cuectx, instance, kubeVersion, tmpDir, cmd.OutOrStdout()); err != nil {
				return err
			}
		}

		elapsed := time.Since(start)
		if bundleApplyArgs.dryrun || bundleApplyArgs.diff {
			log.Info(fmt.Sprintf("applied successfully %s",
				logger.ColorizeDryRun("(server dry run)")))
		} else {
			log.Info(fmt.Sprintf("applied successfully in %s", elapsed.Round(time.Second)))
		}
	}
	return nil
}

func fetchBundleInstanceModule(ctx context.Context, instance *engine.BundleInstance, rootDir string) error {
	modDir := path.Join(rootDir, instance.Name)
	if err := os.MkdirAll(modDir, os.ModePerm); err != nil {
		return err
	}

	moduleVersion := instance.Module.Version
	if moduleVersion == apiv1.LatestVersion && instance.Module.Digest != "" {
		moduleVersion = "@" + instance.Module.Digest
	}

	f, err := fetcher.New(ctx, fetcher.Options{
		Source:      instance.Module.Repository,
		Version:     moduleVersion,
		Destination: modDir,
		CacheDir:    rootArgs.cacheDir,
		Creds:       bundleApplyArgs.creds.String(),
		Insecure:    rootArgs.registryInsecure,
	})
	if err != nil {
		return err
	}

	mod, err := f.Fetch()
	if err != nil {
		return err
	}

	if instance.Module.Digest != "" && mod.Digest != instance.Module.Digest {
		return fmt.Errorf("the upstream digest %s of version %s doesn't match the specified digest %s",
			mod.Digest, instance.Module.Version, instance.Module.Digest)
	}

	instance.Module = *mod
	return nil
}

func applyBundleInstance(ctx context.Context, cuectx *cue.Context, instance *engine.BundleInstance, kubeVersion string, rootDir string, diffOutput io.Writer) error {
	log := loggerBundleInstance(ctx, instance.Bundle, instance.Cluster, instance.Name, true)

	modDir := path.Join(rootDir, instance.Name, "module")
	builder := engine.NewModuleBuilder(
		cuectx,
		instance.Name,
		instance.Namespace,
		modDir,
		bundleApplyArgs.pkg.String(),
	)

	if err := builder.WriteSchemaFile(); err != nil {
		return err
	}

	modName, err := builder.GetModuleName()
	if err != nil {
		return err
	}
	instance.Module.Name = modName

	log.Info(fmt.Sprintf("applying module %s version %s",
		logger.ColorizeSubject(instance.Module.Name), logger.ColorizeSubject(instance.Module.Version)))
	err = builder.WriteValuesFileWithDefaults(instance.Values)
	if err != nil {
		return err
	}

	builder.SetVersionInfo(instance.Module.Version, kubeVersion)

	buildResult, err := builder.Build()
	if err != nil {
		return describeErr(modDir, "build failed for "+instance.Name, err)
	}

	r := reconciler.NewInteractiveReconciler(log,
		&reconciler.CommonOptions{
			Dir:                rootDir,
			Wait:               bundleApplyArgs.wait,
			Force:              bundleApplyArgs.force,
			OverwriteOwnership: bundleApplyArgs.overwriteOwnership,
		},
		&reconciler.InteractiveOptions{
			DryRun:        bundleApplyArgs.dryrun,
			Diff:          bundleApplyArgs.diff,
			DiffOutput:    diffOutput,
			ProgressStart: logger.StartSpinner,
		},
		rootArgs.timeout,
	)

	if err := r.Init(ctx, builder, buildResult, instance, kubeconfigArgs); err != nil {
		return annotateInstanceOwnershipConflictErr(err)
	}

	return r.ApplyInstance(ctx, log,
		builder,
		buildResult,
	)
}

func annotateInstanceOwnershipConflictErr(err error) error {
	if errors.Is(err, &reconciler.InstanceOwnershipConflictErr{}) {
		return fmt.Errorf("%s %s", err, "Apply with \"--overwrite-ownership\" to gain instance ownership.")
	}
	return err
}

func saveReaderToFile(reader io.Reader) (string, error) {
	f, err := os.CreateTemp("", "*.cue")
	if err != nil {
		return "", errors.New("unable to create temp dir for stdin")
	}

	defer f.Close()

	if _, err := io.Copy(f, reader); err != nil {
		return "", fmt.Errorf("error writing stdin to file: %w", err)
	}

	return f.Name(), nil
}

func bundleInstancesOwnershipConflicts(bundleInstances []*engine.BundleInstance) error {
	var conflicts reconciler.InstanceOwnershipConflictErr
	rm, err := runtime.NewResourceManager(kubeconfigArgs)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	sm := runtime.NewStorageManager(rm)
	for _, instance := range bundleInstances {
		if existingInstance, err := sm.Get(ctx, instance.Name, instance.Namespace); err == nil {
			currentOwnerBundle := existingInstance.Labels[apiv1.BundleNameLabelKey]
			if currentOwnerBundle == "" || currentOwnerBundle != instance.Bundle {
				conflicts = append(conflicts, reconciler.InstanceOwnershipConflict{
					InstanceName:       instance.Name,
					CurrentOwnerBundle: currentOwnerBundle,
				})
			}
		}
	}

	if len(conflicts) > 0 {
		return &conflicts
	}
	return nil
}
