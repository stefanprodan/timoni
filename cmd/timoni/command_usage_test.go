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
	"testing"

	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
)

func TestRequiredOperandsAreNotOptionalInUsage(t *testing.T) {
	tests := []struct {
		cmd  *cobra.Command
		want string
	}{
		{applyCmd, "apply INSTANCE_NAME MODULE_URL"},
		{listArtifactCmd, "list ARTIFACT_URL"},
		{pullArtifactCmd, "pull ARTIFACT_URL"},
		{pushArtifactCmd, "push REPOSITORY_URL"},
		{tagArtifactCmd, "tag ARTIFACT_URL"},
		{buildCmd, "build INSTANCE_NAME MODULE_URL"},
		{deleteCmd, "delete INSTANCE_NAME"},
		{inspectModuleCmd, "module INSTANCE_NAME"},
		{inspectResourcesCmd, "resources INSTANCE_NAME"},
		{inspectValuesCmd, "values INSTANCE_NAME"},
		{initModCmd, "init MODULE_NAME [PATH]"},
		{listModCmd, "list MODULE_URL"},
		{pullModCmd, "pull MODULE_URL"},
		{pushModCmd, "push MODULE_PATH MODULE_URL"},
		{statusCmd, "status INSTANCE_NAME"},
	}

	for _, tt := range tests {
		t.Run(tt.cmd.CommandPath(), func(t *testing.T) {
			NewWithT(t).Expect(tt.cmd.Use).To(Equal(tt.want))
		})
	}
}
