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
	"regexp"

	"github.com/spf13/cobra"
)

// validateProviderCompanionFlags checks that companion flags have a provider.
func validateProviderCompanionFlags(cmd *cobra.Command, provider, providerFlag string, companionFlags ...string) error {
	if provider != "" {
		return nil
	}

	for _, name := range companionFlags {
		value, err := cmd.Flags().GetString(name)
		if err != nil {
			return err
		}
		if value != "" {
			return fmt.Errorf("--%s requires --%s", name, providerFlag)
		}
	}

	return nil
}

// validateVerificationFlags checks verification companion flag relationships.
func validateVerificationFlags(cmd *cobra.Command, provider string) error {
	companionFlags := []string{
		"cosign-key", "certificate-identity", "certificate-identity-regexp",
		"certificate-oidc-issuer", "certificate-oidc-issuer-regexp",
	}
	if err := validateProviderCompanionFlags(cmd, provider, "verify", companionFlags...); err != nil {
		return err
	}
	values := make(map[string]string, len(companionFlags))
	for _, name := range companionFlags {
		value, err := cmd.Flags().GetString(name)
		if err != nil {
			return err
		}
		values[name] = value
	}

	conflicts := [][2]string{
		{"certificate-identity", "certificate-identity-regexp"},
		{"certificate-oidc-issuer", "certificate-oidc-issuer-regexp"},
	}
	for _, pair := range conflicts {
		if values[pair[0]] != "" && values[pair[1]] != "" {
			return fmt.Errorf("--%s and --%s are mutually exclusive", pair[0], pair[1])
		}
	}
	if provider != "cosign" {
		return nil
	}

	certificateFlags := companionFlags[1:]
	if values["cosign-key"] != "" {
		for _, name := range certificateFlags {
			if values[name] != "" {
				return fmt.Errorf("--cosign-key cannot be combined with certificate verification flags")
			}
		}
		return nil
	}
	if values["certificate-identity"] == "" && values["certificate-identity-regexp"] == "" {
		return fmt.Errorf("--certificate-identity or --certificate-identity-regexp is required for Cosign verification in keyless mode")
	}
	if values["certificate-oidc-issuer"] == "" && values["certificate-oidc-issuer-regexp"] == "" {
		return fmt.Errorf("--certificate-oidc-issuer or --certificate-oidc-issuer-regexp is required for Cosign verification in keyless mode")
	}
	for _, name := range []string{"certificate-identity-regexp", "certificate-oidc-issuer-regexp"} {
		if value := values[name]; value != "" {
			if _, err := regexp.Compile(value); err != nil {
				return fmt.Errorf("invalid --%s: %w", name, err)
			}
		}
	}

	return nil
}
