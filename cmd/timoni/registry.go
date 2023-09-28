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
	cranecmd "github.com/google/go-containerregistry/cmd/crane/cmd"
	"github.com/spf13/cobra"
)

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Commands for managing the authentication to container registries",
}

func init() {
	rootCmd.AddCommand(registryCmd)
	registryCmd.AddCommand(cranecmd.NewCmdAuthLogin("timoni", "registry"))
	registryCmd.AddCommand(cranecmd.NewCmdAuthLogout("timoni", "registry"))
}
