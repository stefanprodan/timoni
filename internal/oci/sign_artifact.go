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

package oci

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/go-logr/logr"
)

// ValidateSigningProvider reports whether the signing provider is supported
// and available.
func ValidateSigningProvider(provider string) error {
	if provider != "cosign" {
		return fmt.Errorf("signer not supported: %s", provider)
	}
	return validateCosignExecutable()
}

// ValidateVerificationProvider reports whether the verification provider is
// supported and available.
func ValidateVerificationProvider(provider string) error {
	if provider != "cosign" {
		return fmt.Errorf("verifier not supported: %s", provider)
	}
	return validateCosignExecutable()
}

// validateCosignExecutable reports whether Cosign can be executed.
func validateCosignExecutable() error {
	if _, err := exec.LookPath("cosign"); err != nil {
		return fmt.Errorf("executing cosign failed: %w", err)
	}
	return nil
}

// SignArtifact validates the provider and signs an OpenContainers artifact
// with the requested registry transport.
func SignArtifact(ctx context.Context, log logr.Logger, provider string, ociURL string, keyRef string, insecure bool) error {
	ref, err := parseArtifactRef(ociURL)
	if err != nil {
		return err
	}

	if err := ValidateSigningProvider(provider); err != nil {
		return err
	}
	return SignCosign(ctx, log, ref.String(), keyRef, insecure)
}

// VerifyArtifact validates the provider and verifies an OpenContainers artifact
// with the requested registry transport.
func VerifyArtifact(ctx context.Context, log logr.Logger, provider string, ociURL string, keyRef string, certIdentity string, certIdentityRegexp string, certOidcIssuer string, certOidcIssuerRegexp string, insecure bool) error {
	ref, err := parseArtifactRef(ociURL)
	if err != nil {
		return err
	}

	if err := ValidateVerificationProvider(provider); err != nil {
		return err
	}
	return VerifyCosign(ctx, log, ref.String(), keyRef, certIdentity, certIdentityRegexp, certOidcIssuer, certOidcIssuerRegexp, insecure)
}
