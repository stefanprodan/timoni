package signutil

import (
	"fmt"

	"github.com/go-logr/logr"
)

// Sign signs an image using the specified provider.
func Sign(log logr.Logger, provider string, imageRef string, keyRef string) error {
	switch provider {
	case "cosign":
		if err := SignCosign(log, imageRef, keyRef); err != nil {
			return err
		}
	default:
		return fmt.Errorf("no signers found: %s", provider)
	}
	return nil
}

// Verify verifies an image using the specified provider.
func Verify(log logr.Logger, provider string, imageRef string, keyRef string, certIdentity string, certIdentityRegexp string, certOidcIssuer string, certOidcIssuerRegexp string) error {
	switch provider {
	case "cosign":
		if err := VerifyCosign(log, imageRef, keyRef, certIdentity, certIdentityRegexp, certOidcIssuer, certOidcIssuerRegexp); err != nil {
			return err
		}
	default:
		return fmt.Errorf("no verifiers found: %s", provider)
	}
	return nil
}
