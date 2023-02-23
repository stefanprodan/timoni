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

	"github.com/fluxcd/pkg/ssa"
	"github.com/spf13/cobra"
	"github.com/stefanprodan/timoni/internal/runtime"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

var statusCmd = &cobra.Command{
	Use:   "status [INSTANCE NAME]",
	Short: "Displays the current status of Kubernetes resources managed by an instance",
	Example: `  # Show the current status of the managed resources
  timoni status -n apps app
`,
	RunE: runstatusCmd,
}

type statusFlags struct {
	name string
}

var statusArgs statusFlags

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runstatusCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("instance name is required")
	}

	statusArgs.name = args[0]

	rm, err := runtime.NewResourceManager(kubeconfigArgs)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	st := runtime.NewStorageManager(rm)
	inst, err := st.Get(ctx, statusArgs.name, *kubeconfigArgs.Namespace)
	if err != nil {
		return err
	}

	tm := runtime.InstanceManager{Instance: apiv1.Instance{Inventory: inst.Inventory}}

	objects, err := tm.ListObjects()
	if err != nil {
		return err
	}

	for _, obj := range objects {
		err = rm.Client().Get(ctx, client.ObjectKeyFromObject(obj), obj)
		if err != nil {
			if apierrors.IsNotFound(err) {
				logger.Printf("%s NotFound", ssa.FmtUnstructured(obj))
				continue
			}
			logger.Printf("%s %s", ssa.FmtUnstructured(obj), err.Error())
			continue
		}

		res, err := status.Compute(obj)
		if err != nil {
			logger.Printf("%s %s", ssa.FmtUnstructured(obj), err.Error())
			continue
		}
		logger.Printf("%s %s %s", ssa.FmtUnstructured(obj), res.Status, res.Message)
	}

	return nil
}
