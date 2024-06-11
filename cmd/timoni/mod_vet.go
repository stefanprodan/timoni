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
	"fmt"
	"os"
	"path"

	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/pkg/strings"
	ssautil "github.com/fluxcd/pkg/ssa/utils"
	"github.com/google/go-containerregistry/pkg/name"
	cp "github.com/otiai10/copy"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/engine/fetcher"
	"github.com/stefanprodan/timoni/internal/flags"
	"github.com/stefanprodan/timoni/internal/logger"
)

var vetModCmd = &cobra.Command{
	Use:     "vet [MODULE PATH]",
	Aliases: []string{"lint"},
	Short:   "Validate a local module",
	Long:    `The vet command builds the local module and validates the resulting Kubernetes objects.`,
	Example: `  # validate module using default values
  timoni mod vet

  # validate module using debug values
  timoni mod vet ./path/to/module --debug
`,
	RunE: runVetModCmd,
}

type vetModFlags struct {
	path        string
	pkg         flags.Package
	debug       bool
	valuesFiles []string
	name        string
}

var vetModArgs vetModFlags

func init() {
	vetModCmd.Flags().StringVar(&vetModArgs.name, "name", "default", "Name of the instance used to build the module")
	vetModCmd.Flags().VarP(&vetModArgs.pkg, vetModArgs.pkg.Type(), vetModArgs.pkg.Shorthand(), vetModArgs.pkg.Description())
	vetModCmd.Flags().BoolVar(&vetModArgs.debug, "debug", false,
		"Use debug_values.cue if found in the module root instead of the default values.")
	vetModCmd.Flags().StringSliceVarP(&vetModArgs.valuesFiles, "values", "f", nil,
		"The local path to values files (cue, yaml or json format).")
	modCmd.AddCommand(vetModCmd)
}

func runVetModCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		vetModArgs.path = "."
	} else {
		vetModArgs.path = args[0]
	}

	if fs, err := os.Stat(vetModArgs.path); err != nil || !fs.IsDir() {
		return fmt.Errorf("module not found at path %s", vetModArgs.path)
	}

	log := LoggerFrom(cmd.Context())
	cuectx := cuecontext.New()

	tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	ctxPull, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	f, err := fetcher.New(ctxPull, fetcher.Options{
		Source:       vetModArgs.path,
		Version:      apiv1.LatestVersion,
		Destination:  tmpDir,
		CacheDir:     rootArgs.cacheDir,
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

	var tags []string
	if vetModArgs.debug {
		dv := path.Join(vetModArgs.path, "debug_values.cue")
		if _, err := os.Stat(dv); err == nil {
			if cpErr := cp.Copy(dv, path.Join(tmpDir, "module", "debug_values.cue")); cpErr != nil {
				return cpErr
			}
			tags = append(tags, "debug")
			log.Info("vetting with debug values")
		} else {
			log.Info("vetting with default values (debug values not found)")
		}
	} else {
		log.Info("vetting with default values")
	}

	builder := engine.NewModuleBuilder(
		cuectx,
		vetModArgs.name,
		*kubeconfigArgs.Namespace,
		f.GetModuleRoot(),
		vetModArgs.pkg.String(),
	)

	if err := builder.WriteSchemaFile(); err != nil {
		return err
	}

	mod.Name, err = builder.GetModuleName()
	if err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	if len(vetModArgs.valuesFiles) > 0 {
		valuesCue, err := convertToCue(cmd, vetModArgs.valuesFiles)
		if err != nil {
			return err
		}
		err = builder.MergeValuesFile(valuesCue)
		if err != nil {
			return err
		}
	}

	buildResult, err := builder.Build(tags...)
	if err != nil {
		return describeErr(f.GetModuleRoot(), "validation failed", err)
	}

	applySets, err := builder.GetApplySets(buildResult)
	if err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	if len(applySets) == 0 {
		return fmt.Errorf("%s contains no objects", apiv1.ApplySelector)
	}

	var objects []*unstructured.Unstructured
	for _, set := range applySets {
		objects = append(objects, set.Objects...)
	}

	if len(objects) == 0 {
		return fmt.Errorf("build failed, no objects to apply")
	}

	for _, object := range objects {
		log.Info(fmt.Sprintf("%s %s",
			logger.ColorizeSubject(ssautil.FmtUnstructured(object)), logger.ColorizeInfo("valid resource")))
	}

	images, err := builder.GetContainerImages(buildResult)
	if err != nil {
		return fmt.Errorf("failed to extract images: %w", err)
	}

	for _, image := range images {
		if _, err := name.ParseReference(image); err != nil {
			log.Error(err, "invalid image")
			continue
		}

		if !strings.Contains(image, "@sha") {
			log.Info(fmt.Sprintf("%s %s",
				logger.ColorizeSubject(image), logger.ColorizeWarning("valid image (digest missing)")))
		} else {
			log.Info(fmt.Sprintf("%s %s",
				logger.ColorizeSubject(image), logger.ColorizeInfo("valid image")))
		}
	}

	log.Info(fmt.Sprintf("%s %s",
		logger.ColorizeSubject(mod.Name), logger.ColorizeInfo("valid module")))

	return nil
}
