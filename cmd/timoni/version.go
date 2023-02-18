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
	"encoding/json"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

var versionCmd = &cobra.Command{
	Use:     "version",
	Short:   "Print the client and API version information.",
	Example: "timoni version -o yaml",
	RunE:    runVersionCmd,
}

type versionFlags struct {
	output string
}

var versionArgs versionFlags

func init() {
	versionCmd.Flags().StringVarP(&versionArgs.output, "output", "o", "yaml",
		"The format in which the version information should be printed, can be 'yaml' or 'json'")
	rootCmd.AddCommand(versionCmd)
}

func runVersionCmd(cmd *cobra.Command, args []string) error {
	info := map[string]string{}
	info["client"] = VERSION
	info["api"] = apiv1.GroupVersion.String()

	var marshalled []byte
	var err error

	if versionArgs.output == "json" {
		marshalled, err = json.MarshalIndent(&info, "", "  ")
		marshalled = append(marshalled, "\n"...)
	} else {
		marshalled, err = yaml.Marshal(&info)
	}

	if err != nil {
		return err
	}

	cmd.OutOrStdout().Write(marshalled)

	return nil
}
