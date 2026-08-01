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
	"fmt"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
)

func TestPushCommandsRequireSigningProvider(t *testing.T) {
	tests := []string{
		"artifact push oci://example.com/org/artifact --tag 1.0.0 --filepath /does/not/exist --cosign-key key",
		"mod push /does/not/exist oci://example.com/org/module --version 1.0.0 --cosign-key key",
	}

	for _, command := range tests {
		_, err := executeCommand(command)
		NewWithT(t).Expect(err).To(MatchError("--cosign-key requires --sign"))
	}
}

func TestValidateVerificationFlagsAcceptsValidModes(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "key", args: []string{"--cosign-key", "key"}},
		{name: "keyless", args: []string{"--certificate-identity", "user@example.com", "--certificate-oidc-issuer", "issuer"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			for _, name := range []string{
				"cosign-key", "certificate-identity", "certificate-identity-regexp",
				"certificate-oidc-issuer", "certificate-oidc-issuer-regexp",
			} {
				cmd.Flags().String(name, "", "")
			}
			NewWithT(t).Expect(cmd.Flags().Parse(tt.args)).To(Succeed())
			NewWithT(t).Expect(validateVerificationFlags(cmd, "cosign")).To(Succeed())
		})
	}
}

func TestPullCommandsValidateVerificationFlags(t *testing.T) {
	tests := []struct {
		name    string
		command string
		err     string
	}{
		{
			name:    "artifact companion",
			command: "artifact pull oci://example.com/org/artifact --output %s --certificate-identity user@example.com",
			err:     "--certificate-identity requires --verify",
		},
		{
			name:    "module companion",
			command: "mod pull oci://example.com/org/module --output %s --cosign-key key",
			err:     "--cosign-key requires --verify",
		},
		{
			name:    "artifact identity conflict",
			command: "artifact pull oci://example.com/org/artifact --output %s --verify cosign --certificate-identity user@example.com --certificate-identity-regexp example.com",
			err:     "--certificate-identity and --certificate-identity-regexp are mutually exclusive",
		},
		{
			name:    "artifact issuer conflict",
			command: "artifact pull oci://example.com/org/artifact --output %s --verify cosign --certificate-oidc-issuer issuer --certificate-oidc-issuer-regexp issuer",
			err:     "--certificate-oidc-issuer and --certificate-oidc-issuer-regexp are mutually exclusive",
		},
		{
			name:    "module identity conflict",
			command: "mod pull oci://example.com/org/module --output %s --verify cosign --certificate-identity user@example.com --certificate-identity-regexp example.com",
			err:     "--certificate-identity and --certificate-identity-regexp are mutually exclusive",
		},
		{
			name:    "module issuer conflict",
			command: "mod pull oci://example.com/org/module --output %s --verify cosign --certificate-oidc-issuer issuer --certificate-oidc-issuer-regexp issuer",
			err:     "--certificate-oidc-issuer and --certificate-oidc-issuer-regexp are mutually exclusive",
		},
		{
			name:    "artifact keyless identity required",
			command: "artifact pull oci://example.com/org/artifact --output %s --verify cosign --certificate-oidc-issuer issuer",
			err:     "--certificate-identity or --certificate-identity-regexp is required for Cosign verification in keyless mode",
		},
		{
			name:    "module keyless issuer required",
			command: "mod pull oci://example.com/org/module --output %s --verify cosign --certificate-identity user@example.com",
			err:     "--certificate-oidc-issuer or --certificate-oidc-issuer-regexp is required for Cosign verification in keyless mode",
		},
		{
			name:    "artifact key and certificate conflict",
			command: "artifact pull oci://example.com/org/artifact --output %s --verify cosign --cosign-key key --certificate-identity user@example.com",
			err:     "--cosign-key cannot be combined with certificate verification flags",
		},
		{
			name:    "module key and certificate conflict",
			command: "mod pull oci://example.com/org/module --output %s --verify cosign --cosign-key key --certificate-oidc-issuer issuer",
			err:     "--cosign-key cannot be combined with certificate verification flags",
		},
		{
			name:    "artifact invalid identity regexp",
			command: "artifact pull oci://example.com/org/artifact --output %s --verify cosign --certificate-identity-regexp '[' --certificate-oidc-issuer issuer",
			err:     "invalid --certificate-identity-regexp",
		},
		{
			name:    "module invalid issuer regexp",
			command: "mod pull oci://example.com/org/module --output %s --verify cosign --certificate-identity user@example.com --certificate-oidc-issuer-regexp '['",
			err:     "invalid --certificate-oidc-issuer-regexp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			output := filepath.Join(t.TempDir(), "output")

			_, err := executeCommand(fmt.Sprintf(tt.command, output))
			g.Expect(err).To(MatchError(ContainSubstring(tt.err)))
			g.Expect(output).ToNot(Or(BeADirectory(), BeAnExistingFile()))
		})
	}
}
