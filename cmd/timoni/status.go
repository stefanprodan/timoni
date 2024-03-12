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

	"github.com/fluxcd/cli-utils/pkg/kstatus/status"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stefanprodan/timoni/internal/logger"
	"github.com/stefanprodan/timoni/internal/runtime"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

var statusCmd = &cobra.Command{
	Use:   "status [INSTANCE NAME]",
	Short: "Displays the current status of Kubernetes resources managed by an instance",
	Example: `  # Show the current status of the managed resources
  timoni -n apps status app
`,
	RunE: runStatusCmd,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		switch len(args) {
		case 0:
			return completeInstanceList(cmd, args, toComplete)
		default:
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
	},
}

type statusFlags struct {
	name string
}

var statusArgs statusFlags

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatusCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("instance name is required")
	}

	statusArgs.name = args[0]

	log := logger.LoggerInstance(cmd.Context(), statusArgs.name, true)
	rm, err := runtime.NewResourceManager(kubeconfigArgs)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	st := runtime.NewStorageManager(rm)
	instance, err := st.Get(ctx, statusArgs.name, *kubeconfigArgs.Namespace)
	if err != nil {
		return err
	}

	log.Info(fmt.Sprintf("last applied %s",
		logger.ColorizeSubject(instance.LastTransitionTime)))
	log.Info(fmt.Sprintf("module %s",
		logger.ColorizeSubject(instance.Module.Repository+":"+instance.Module.Version)))
	log.Info(fmt.Sprintf("digest %s",
		logger.ColorizeSubject(instance.Module.Digest)))

	for _, image := range instance.Images {
		log.Info(fmt.Sprintf("container image %s",
			logger.ColorizeSubject(image)))
	}

	tm := runtime.InstanceManager{Instance: apiv1.Instance{Inventory: instance.Inventory}}

	objects, err := tm.ListObjects()
	if err != nil {
		return err
	}

	for _, obj := range objects {
		err = rm.Client().Get(ctx, client.ObjectKeyFromObject(obj), obj)
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Error(err, logger.ColorizeJoin(obj, errors.New("NotFound")))
				continue
			}
			log.Error(err, logger.ColorizeJoin(obj, errors.New("Unknown")))
			continue
		}

		res, err := status.Compute(obj)
		if err != nil {
			log.Error(err, logger.ColorizeJoin(obj, errors.New("Failed")))
			continue
		}
		log.Info(logger.ColorizeJoin(obj, res.Status, "-", res.Message))
	}

	return nil
}
