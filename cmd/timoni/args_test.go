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

func TestCommandsRejectExtraArgs(t *testing.T) {
	tests := []struct {
		name string
		cmd  *cobra.Command
		args []string
	}{
		{name: "apply", cmd: applyCmd, args: []string{"instance", "module", "extra"}},
		{name: "artifact digest", cmd: digestArtifactCmd, args: []string{"url", "extra"}},
		{name: "artifact list", cmd: listArtifactCmd, args: []string{"url", "extra"}},
		{name: "artifact pull", cmd: pullArtifactCmd, args: []string{"url", "extra"}},
		{name: "artifact push", cmd: pushArtifactCmd, args: []string{"url", "extra"}},
		{name: "build", cmd: buildCmd, args: []string{"instance", "module", "extra"}},
		{name: "bundle status", cmd: bundleStatusCmd, args: []string{"bundle", "extra"}},
		{name: "completion bash", cmd: completionBashCmd, args: []string{"extra"}},
		{name: "completion fish", cmd: completionFishCmd, args: []string{"extra"}},
		{name: "completion powershell", cmd: completionPowerShellCmd, args: []string{"extra"}},
		{name: "completion zsh", cmd: completionZshCmd, args: []string{"extra"}},
		{name: "delete", cmd: deleteCmd, args: []string{"instance", "extra"}},
		{name: "docgen", cmd: docgenCmd, args: []string{"extra"}},
		{name: "inspect module", cmd: inspectModuleCmd, args: []string{"instance", "extra"}},
		{name: "inspect resources", cmd: inspectResourcesCmd, args: []string{"instance", "extra"}},
		{name: "inspect values", cmd: inspectValuesCmd, args: []string{"instance", "extra"}},
		{name: "list", cmd: listCmd, args: []string{"extra"}},
		{name: "mod init", cmd: initModCmd, args: []string{"module", "path", "extra"}},
		{name: "mod list", cmd: listModCmd, args: []string{"url", "extra"}},
		{name: "mod pull", cmd: pullModCmd, args: []string{"url", "extra"}},
		{name: "mod push", cmd: pushModCmd, args: []string{"module", "url", "extra"}},
		{name: "mod show config", cmd: configShowModCmd, args: []string{"module", "extra"}},
		{name: "mod vendor crd", cmd: vendorCrdCmd, args: []string{"module", "extra"}},
		{name: "mod vendor k8s", cmd: vendorK8sCmd, args: []string{"module", "extra"}},
		{name: "mod vet", cmd: vetModCmd, args: []string{"module", "extra"}},
		{name: "status", cmd: statusCmd, args: []string{"instance", "extra"}},
		{name: "version", cmd: versionCmd, args: []string{"extra"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(tt.cmd.Args).NotTo(BeNil())
			g.Expect(tt.cmd.Args(tt.cmd, nil)).To(Succeed())
			g.Expect(tt.cmd.Args(tt.cmd, tt.args[:len(tt.args)-1])).To(Succeed())
			wantErr := "accepts at most"
			if len(tt.args) == 1 {
				wantErr = "unknown command"
			}
			g.Expect(tt.cmd.Args(tt.cmd, tt.args)).To(MatchError(ContainSubstring(wantErr)))
		})
	}
}
