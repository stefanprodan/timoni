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

package signutil

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/go-logr/logr"
)

// Sign signs an image (`imageRef`) using a cosign private key (`keyRef`)
func Sign(log logr.Logger, imageRef string, keyRef string) error {
	cosignExecutable, err := exec.LookPath("cosign")
	if err != nil {
		return fmt.Errorf("executing cosign failed: %w", err)
	}

	cosignCmd := exec.Command(cosignExecutable, []string{"sign"}...)
	cosignCmd.Env = os.Environ()

	// if key is empty, use keyless mode
	if keyRef != "" {
		cosignCmd.Args = append(cosignCmd.Args, "--key", keyRef)
	}

	cosignCmd.Args = append(cosignCmd.Args, "--yes")
	cosignCmd.Args = append(cosignCmd.Args, imageRef)

	err = processCosignIO(log, cosignCmd)
	if err != nil {
		return err
	}

	return cosignCmd.Wait()
}

// Verify verifies an image (`rawRef`) with a cosign public key (`keyRef`)
// Either --cosign-certificate-identity or --cosign-certificate-identity-regexp and either --cosign-certificate-oidc-issuer or --cosign-certificate-oidc-issuer-regexp must be set for keyless flows.
func Verify(log logr.Logger, imageRef string, keyRef string,
	certIdentity string, certIdentityRegexp string, certOidcIssuer string, certOidcIssuerRegexp string) error {
	cosignExecutable, err := exec.LookPath("cosign")
	if err != nil {
		return fmt.Errorf("executing cosign failed: %w", err)
	}

	cosignCmd := exec.Command(cosignExecutable, []string{"verify"}...)
	cosignCmd.Env = os.Environ()

	// if key is empty, use keyless mode
	if keyRef != "" {
		cosignCmd.Args = append(cosignCmd.Args, "--key", keyRef)
	} else {
		if certIdentity == "" && certIdentityRegexp == "" {
			return errors.New("--certificate-identity or --certificate-identity-regexp is required for Cosign verification in keyless mode")
		}
		if certIdentity != "" {
			cosignCmd.Args = append(cosignCmd.Args, "--certificate-identity", certIdentity)
		}
		if certIdentityRegexp != "" {
			cosignCmd.Args = append(cosignCmd.Args, "--certificate-identity-regexp", certIdentityRegexp)
		}
		if certOidcIssuer == "" && certOidcIssuerRegexp == "" {
			return errors.New("--certificate-oidc-issuer or --certificate-oidc-issuer-regexp is required for Cosign verification in keyless mode")
		}
		if certOidcIssuer != "" {
			cosignCmd.Args = append(cosignCmd.Args, "--certificate-oidc-issuer", certOidcIssuer)
		}
		if certOidcIssuerRegexp != "" {
			cosignCmd.Args = append(cosignCmd.Args, "--certificate-oidc-issuer-regexp", certOidcIssuerRegexp)
		}
	}

	cosignCmd.Args = append(cosignCmd.Args, imageRef)

	err = processCosignIO(log, cosignCmd)
	if err != nil {
		return err
	}
	if err := cosignCmd.Wait(); err != nil {
		return err
	}

	return nil
}

func processCosignIO(log logr.Logger, cosignCmd *exec.Cmd) error {
	stdout, err := cosignCmd.StdoutPipe()
	if err != nil {
		log.Error(err, "cosign stdout pipe failed")
	}
	stderr, err := cosignCmd.StderrPipe()
	if err != nil {
		log.Error(err, "cosign stderr pipe failed")
	}

	merged := io.MultiReader(stdout, stderr)
	scanner := bufio.NewScanner(merged)

	if err := cosignCmd.Start(); err != nil {
		return fmt.Errorf("executing cosign failed: %w", err)
	}

	for scanner.Scan() {
		log.Info("cosign: " + scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		log.Error(err, "cosign stdout/stderr scanner failed")
	}

	return nil
}
