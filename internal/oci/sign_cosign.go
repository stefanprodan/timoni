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
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/go-logr/logr"
)

const (
	maxCosignOutputLogLine   = 1024 * 1024
	maxCosignOutputScanToken = maxCosignOutputLogLine + 1
)

// SignCosign signs an image and optionally permits insecure registry transport.
func SignCosign(ctx context.Context, log logr.Logger, imageRef string, keyRef string, insecure bool) error {
	cosignExecutable, err := exec.LookPath("cosign")
	if err != nil {
		return fmt.Errorf("executing cosign failed: %w", err)
	}

	cosignCmd := newCosignCommand(ctx, cosignExecutable, "sign", insecure)
	cosignCmd.Env = os.Environ()

	// if key is empty, use keyless mode
	if keyRef != "" {
		cosignCmd.Args = append(cosignCmd.Args, "--key", keyRef)
	}

	cosignCmd.Args = append(cosignCmd.Args, "--yes")
	cosignCmd.Args = append(cosignCmd.Args, imageRef)

	err = processCosignIO(ctx, log, cosignCmd)
	if err != nil {
		return err
	}

	return nil
}

// VerifyCosign verifies an image and optionally permits insecure registry
// transport. Keyless flows require an identity and an OIDC issuer or their
// regular-expression alternatives.
func VerifyCosign(ctx context.Context, log logr.Logger, imageRef string, keyRef string,
	certIdentity string, certIdentityRegexp string, certOidcIssuer string, certOidcIssuerRegexp string,
	insecure bool) error {
	cosignExecutable, err := exec.LookPath("cosign")
	if err != nil {
		return fmt.Errorf("executing cosign failed: %w", err)
	}

	cosignCmd := newCosignCommand(ctx, cosignExecutable, "verify", insecure)
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

	err = processCosignIO(ctx, log, cosignCmd)
	if err != nil {
		return err
	}

	return nil
}

// newCosignCommand creates a Cosign command with the requested registry transport.
func newCosignCommand(ctx context.Context, executable string, operation string, insecure bool) *exec.Cmd {
	args := []string{operation}
	if insecure {
		args = append(args, "--allow-http-registry", "--allow-insecure-registry")
	}
	return exec.CommandContext(ctx, executable, args...)
}

// processCosignIO runs cosign and logs its output while draining both streams.
func processCosignIO(ctx context.Context, log logr.Logger, cosignCmd *exec.Cmd) error {
	stdout, err := cosignCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("cosign stdout pipe failed: %w", err)
	}
	stderr, err := cosignCmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("cosign stderr pipe failed: %w", err)
	}

	if err := cosignCmd.Start(); err != nil {
		return fmt.Errorf("executing cosign failed: %w", err)
	}

	outputErrs := make(chan error, 2)
	go func() {
		outputErrs <- scanCosignOutput(log, stdout)
	}()
	go func() {
		outputErrs <- scanCosignOutput(log, stderr)
	}()
	stopClosing := make(chan struct{})
	pipesClosed := make(chan struct{})
	go func() {
		defer close(pipesClosed)
		select {
		case <-ctx.Done():
			_ = stdout.Close()
			_ = stderr.Close()
		case <-stopClosing:
		}
	}()
	stdoutErr := <-outputErrs
	stderrErr := <-outputErrs
	close(stopClosing)
	<-pipesClosed

	if err := errors.Join(stdoutErr, stderrErr); err != nil {
		log.Error(err, "cosign output could not be fully logged")
	}
	if err := cosignCmd.Wait(); err != nil {
		return errors.Join(ctx.Err(), err)
	}
	return nil
}

// scanCosignOutput logs one cosign output stream and drains it after errors.
func scanCosignOutput(log logr.Logger, output io.Reader) error {
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), maxCosignOutputScanToken)
	for scanner.Scan() {
		log.Info("cosign: " + scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, os.ErrClosed) {
			return nil
		}
		_, drainErr := io.Copy(io.Discard, output)
		return errors.Join(err, drainErr)
	}
	return nil
}
