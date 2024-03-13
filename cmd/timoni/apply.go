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
	"os"
	"strings"

	"cuelang.org/go/cue/cuecontext"
	"github.com/spf13/cobra"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/apply"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/engine/fetcher"
	"github.com/stefanprodan/timoni/internal/flags"
	"github.com/stefanprodan/timoni/internal/runtime"
)

var applyCmd = &cobra.Command{
	Use:     "apply [INSTANCE NAME] [MODULE URL]",
	Aliases: []string{"install", "upgrade"},
	Short:   "Install or upgrade a module instance",
	Long: `The apply command installs or upgrades a module instance on the Kubernetes cluster.

The apply command performs the following steps:

- Pulls the module version from the specified container registry.
- If the registry is private, uses the credentials found in '~/.docker/config.json'.
- If the registry credentials are specified with '--creds', these take priority over the docker ones.
- Creates the specified '--namespace' if it doesn't exist.
- Merges all the values supplied with '--values' on top of the default values found in the module.
- Builds the module by passing the instance name, namespace and values.
- Labels the resulting Kubernetes resources with the instance name and namespace.
- Applies the Kubernetes resources on the cluster.
- Creates or updates the instance inventory with the last applied resources IDs (stored in a secret named timoni.<instance_name>).
- Recreates the resources annotated with 'action.timoni.sh/force: "enabled"' if they contain changes to immutable fields.
- Waits for the applied resources to become ready.
- Deletes the resources which were previously applied but are missing from the current instance.
- Skips the resources annotated with 'action.timoni.sh/prune: "disabled"' from deletion.
- Waits for the deleted resources to be finalised.
`,
	Example: `  # Install a module instance and create the namespace if it doesn't exists
  timoni apply -n apps app oci://docker.io/org/module -v 1.0.0

  # Do a dry-run upgrade and print the diff
  timoni apply -n apps app oci://docker.io/org/module -v 1.0.0 \
  --values ./values-1.cue \
  --dry-run --diff

  # Install or upgrade an instance with custom values by merging them in the specified order
  timoni apply -n apps app oci://docker.io/org/module -v 1.0.0 \
  --values ./values-1.cue \
  --values ./values-2.cue

  # Upgrade an instance and recreate immutable Kubernetes resources such as Jobs
  timoni apply -n apps app oci://docker.io/org/module -v 2.0.0 \
  --values ./values-1.cue \
  --force

  # Install or upgrade an instance with custom values from stdin
  echo "values: replicas: 2" | timoni apply -n apps app oci://docker.io/org/module --values -

  # Install or upgrade an instance with values in YAML and JSON format
  timoni apply -n apps app oci://docker.io/org/module \
  --values ./values-1.yaml \
  --values ./values-2.json
`,
	RunE: runApplyCmd,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		switch len(args) {
		case 0:
			return completeInstanceList(cmd, args, toComplete)
		case 1:
			return nil, cobra.ShellCompDirectiveFilterDirs
		default:
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
	},
}

type applyFlags struct {
	name               string
	module             string
	version            flags.Version
	pkg                flags.Package
	valuesFiles        []string
	dryrun             bool
	diff               bool
	wait               bool
	force              bool
	overwriteOwnership bool
	creds              flags.Credentials
}

var applyArgs applyFlags

const ownershipConflictHint = "Apply with \"--overwrite-ownership\" to gain instance ownership."

func init() {
	applyCmd.Flags().VarP(&applyArgs.version, applyArgs.version.Type(), applyArgs.version.Shorthand(), applyArgs.version.Description())
	applyCmd.Flags().VarP(&applyArgs.pkg, applyArgs.pkg.Type(), applyArgs.pkg.Shorthand(), applyArgs.pkg.Description())
	applyCmd.Flags().StringSliceVarP(&applyArgs.valuesFiles, "values", "f", nil,
		"The local path to values files (cue, yaml or json format).")
	applyCmd.Flags().BoolVar(&applyArgs.force, "force", false,
		"Recreate immutable Kubernetes resources.")
	applyCmd.Flags().BoolVar(&applyArgs.overwriteOwnership, "overwrite-ownership", false,
		"Overwrite instance ownership, if the instance is owned by a Bundle.")
	applyCmd.Flags().BoolVar(&applyArgs.dryrun, "dry-run", false,
		"Perform a server-side apply dry run.")
	applyCmd.Flags().BoolVar(&applyArgs.diff, "diff", false,
		"Perform a server-side apply dry run and prints the diff.")
	applyCmd.Flags().BoolVar(&applyArgs.wait, "wait", true,
		"Wait for the applied Kubernetes objects to become ready.")
	applyCmd.Flags().Var(&applyArgs.creds, applyArgs.creds.Type(), applyArgs.creds.Description())
	rootCmd.AddCommand(applyCmd)
}

func runApplyCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 2 {
		return errors.New("name and module are required")
	}

	applyArgs.name = args[0]
	applyArgs.module = args[1]

	log := loggerInstance(cmd.Context(), applyArgs.name, true)

	version := applyArgs.version.String()
	if version == "" {
		version = apiv1.LatestVersion
	}

	if strings.HasPrefix(applyArgs.module, apiv1.ArtifactPrefix) {
		log.Info(fmt.Sprintf("pulling %s:%s", applyArgs.module, version))
	} else {
		log.Info(fmt.Sprintf("building %s", applyArgs.module))
	}

	tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	ctxPull, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	f, err := fetcher.New(ctxPull, fetcher.Options{
		Source:       applyArgs.module,
		Version:      version,
		Destination:  tmpDir,
		CacheDir:     rootArgs.cacheDir,
		Creds:        applyArgs.creds.String(),
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

	cuectx := cuecontext.New()
	builder := engine.NewModuleBuilder(
		cuectx,
		applyArgs.name,
		*kubeconfigArgs.Namespace,
		f.GetModuleRoot(),
		applyArgs.pkg.String(),
	)

	if err := builder.WriteSchemaFile(); err != nil {
		return err
	}

	mod.Name, err = builder.GetModuleName()
	if err != nil {
		return err
	}

	log.Info(fmt.Sprintf("using module %s version %s", mod.Name, mod.Version))

	if len(applyArgs.valuesFiles) > 0 {
		valuesCue, err := convertToCue(cmd, applyArgs.valuesFiles)
		if err != nil {
			return err
		}
		err = builder.MergeValuesFile(valuesCue)
		if err != nil {
			return err
		}
	}

	kubeVersion, err := runtime.ServerVersion(kubeconfigArgs)
	if err != nil {
		return err
	}

	builder.SetVersionInfo(mod.Version, kubeVersion)

	buildResult, err := builder.Build()
	if err != nil {
		return describeErr(f.GetModuleRoot(), "build failed", err)
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), rootArgs.timeout)
	defer cancel()

	opts := apply.Options{
		Dir:                   tmpDir,
		DryRun:                applyArgs.dryrun,
		Diff:                  applyArgs.diff,
		Wait:                  applyArgs.wait,
		Force:                 applyArgs.force,
		OverwriteOwnership:    applyArgs.overwriteOwnership,
		DiffOutput:            cmd.OutOrStdout(),
		KubeConfigFlags:       kubeconfigArgs,
		OwnershipConflictHint: ownershipConflictHint,
		// ProgressStart:      logger.StartSpinner,
	}

	bi := &engine.BundleInstance{
		Name:      applyArgs.name,
		Namespace: *kubeconfigArgs.Namespace,
		Module:    *mod,
		Bundle:    "",
	}

	return apply.ApplyInstance(ctx, log, builder, buildResult, bi, opts, rootArgs.timeout)
}
