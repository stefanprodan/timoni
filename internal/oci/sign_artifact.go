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
	"fmt"

	"github.com/go-logr/logr"
)

// ValidateSigningProvider reports whether the signing provider is supported.
func ValidateSigningProvider(provider string) error {
	if provider != "cosign" {
		return fmt.Errorf("signer not supported: %s", provider)
	}
	return nil
}

// SignArtifact validates the provider and signs an OpenContainers artifact.
func SignArtifact(log logr.Logger, provider string, ociURL string, keyRef string) error {
	ref, err := parseArtifactRef(ociURL)
	if err != nil {
		return err
	}

	if err := ValidateSigningProvider(provider); err != nil {
		return err
	}
	return SignCosign(log, ref.String(), keyRef)
}

// VerifyArtifact verifies an OpenContainers artifact using the specified provider.
func VerifyArtifact(log logr.Logger, provider string, ociURL string, keyRef string, certIdentity string, certIdentityRegexp string, certOidcIssuer string, certOidcIssuerRegexp string) error {
	ref, err := parseArtifactRef(ociURL)
	if err != nil {
		return err
	}

	switch provider {
	case "cosign":
		if err := VerifyCosign(log, ref.String(), keyRef, certIdentity, certIdentityRegexp, certOidcIssuer, certOidcIssuerRegexp); err != nil {
			return err
		}
	default:
		return fmt.Errorf("verifier not supported: %s", provider)
	}
	return nil
}
