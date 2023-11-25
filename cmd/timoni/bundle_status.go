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

	"cuelang.org/go/cue/cuecontext"
	"github.com/fluxcd/cli-utils/pkg/kstatus/status"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/runtime"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

var bundleStatusCmd = &cobra.Command{
	Use:   "status [BUNDLE NAME]",
	Short: "Displays the current status of Kubernetes resources managed by the bundle instances",
	Example: `  # Show the status of the resources managed by a bundle
  timoni bundle status -f bundle.cue

  # Show the status using a named bundle
  timoni bundle status my-app
`,
	RunE: runBundleStatusCmd,
}

type bundleStatusFlags struct {
	name     string
	filename string
}

var bundleStatusArgs bundleStatusFlags

func init() {
	bundleStatusCmd.Flags().StringVarP(&bundleStatusArgs.filename, "file", "f", "",
		"The local path to bundle.cue file.")
	bundleCmd.AddCommand(bundleStatusCmd)
}

func runBundleStatusCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 1 && bundleStatusArgs.filename == "" {
		return fmt.Errorf("bundle name is required")
	}

	switch {
	case bundleStatusArgs.filename != "":
		cuectx := cuecontext.New()
		name, err := engine.ExtractStringFromFile(cuectx, bundleStatusArgs.filename, apiv1.BundleName.String())
		if err != nil {
			return err
		}
		bundleStatusArgs.name = name
	default:
		bundleStatusArgs.name = args[0]
	}

	rt, err := buildRuntime(bundleArgs.runtimeFiles)
	if err != nil {
		return err
	}

	clusters := rt.SelectClusters(bundleArgs.runtimeCluster, bundleArgs.runtimeClusterGroup)
	if len(clusters) == 0 {
		return fmt.Errorf("no cluster found")
	}

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	failed := false
	for _, cluster := range clusters {
		kubeconfigArgs.Context = &cluster.KubeContext

		rm, err := runtime.NewResourceManager(kubeconfigArgs)
		if err != nil {
			return err
		}

		sm := runtime.NewStorageManager(rm)
		instances, err := sm.List(ctx, "", bundleStatusArgs.name)
		if err != nil {
			return err
		}

		log := LoggerBundle(ctx, bundleStatusArgs.name, cluster.Name)

		if len(instances) == 0 {
			log.Error(nil, "no instances found in bundle")
			failed = true
			continue
		}

		for _, instance := range instances {
			log := LoggerBundleInstance(ctx, bundleStatusArgs.name, cluster.Name, instance.Name)

			log.Info(fmt.Sprintf("last applied %s",
				colorizeSubject(instance.LastTransitionTime)))
			log.Info(fmt.Sprintf("module %s",
				colorizeSubject(instance.Module.Repository+":"+instance.Module.Version)))
			log.Info(fmt.Sprintf("digest %s",
				colorizeSubject(instance.Module.Digest)))

			for _, image := range instance.Images {
				log.Info(fmt.Sprintf("container image %s",
					colorizeSubject(image)))
			}

			im := runtime.InstanceManager{Instance: apiv1.Instance{Inventory: instance.Inventory}}

			objects, err := im.ListObjects()
			if err != nil {
				return err
			}

			for _, obj := range objects {
				err = rm.Client().Get(ctx, client.ObjectKeyFromObject(obj), obj)
				if err != nil {
					if apierrors.IsNotFound(err) {
						log.Error(err, colorizeJoin(obj, errors.New("NotFound")))
						failed = true

						continue
					}
					log.Error(err, colorizeJoin(obj, errors.New("Unknown")))
					failed = true
					continue
				}

				res, err := status.Compute(obj)
				if err != nil {
					log.Error(err, colorizeJoin(obj, errors.New("Failed")))
					failed = true
					continue
				}
				log.Info(colorizeJoin(obj, res.Status, "-", res.Message))
			}
		}
	}
	if failed {
		return fmt.Errorf("completed with errors")
	}
	return nil
}
