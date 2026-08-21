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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
)

const (
	maxCosignOutputLogLine   = 1024 * 1024
	maxCosignOutputScanToken = maxCosignOutputLogLine + 1
)

// dockerConfig contains registry authentication for a Cosign subprocess.
type dockerConfig struct {
	Auths map[string]dockerAuthConfig `json:"auths"`
}

// dockerAuthConfig contains one registry's Docker-compatible authentication.
type dockerAuthConfig struct {
	Auth          string `json:"auth,omitempty"`
	RegistryToken string `json:"registrytoken,omitempty"`
}

// SignCosign signs an image with the requested registry credentials and transport.
func SignCosign(ctx context.Context, log logr.Logger, imageRef string, keyRef string, insecure bool, credentials string) (retErr error) {
	cosignExecutable, err := exec.LookPath("cosign")
	if err != nil {
		return fmt.Errorf("executing cosign failed: %w", err)
	}

	cosignCmd := newCosignCommand(ctx, cosignExecutable, "sign", insecure)
	cosignCmd.Env = os.Environ()
	cleanup, err := configureCosignCredentials(cosignCmd, imageRef, credentials)
	if err != nil {
		return err
	}
	defer func() {
		retErr = errors.Join(retErr, cleanup())
	}()

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
	insecure bool, credentials string) (retErr error) {
	cosignExecutable, err := exec.LookPath("cosign")
	if err != nil {
		return fmt.Errorf("executing cosign failed: %w", err)
	}

	cosignCmd := newCosignCommand(ctx, cosignExecutable, "verify", insecure)
	cosignCmd.Env = os.Environ()
	cleanup, err := configureCosignCredentials(cosignCmd, imageRef, credentials)
	if err != nil {
		return err
	}
	defer func() {
		retErr = errors.Join(retErr, cleanup())
	}()

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

// configureCosignCredentials gives a Cosign subprocess an isolated Docker
// config and returns a function that removes it.
func configureCosignCredentials(cosignCmd *exec.Cmd, imageRef string, credentials string) (func() error, error) {
	cleanup := func() error { return nil }
	if credentials == "" {
		return cleanup, nil
	}

	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return cleanup, fmt.Errorf("parsing artifact reference failed: %w", err)
	}
	authKey := ref.Context().RegistryStr()
	if authKey == name.DefaultRegistry {
		authKey = authn.DefaultAuthKey
	}

	authConfig := dockerAuthConfig{}
	parts := strings.SplitN(credentials, ":", 2)
	if len(parts) == 1 {
		authConfig.RegistryToken = parts[0]
	} else {
		authConfig.Auth = base64.StdEncoding.EncodeToString([]byte(credentials))
	}
	configData, err := json.Marshal(dockerConfig{Auths: map[string]dockerAuthConfig{authKey: authConfig}})
	if err != nil {
		return cleanup, fmt.Errorf("encoding Cosign registry credentials failed: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "timoni-cosign-")
	if err != nil {
		return cleanup, fmt.Errorf("creating Cosign registry config failed: %w", err)
	}
	cleanup = func() error { return os.RemoveAll(tempDir) }
	dockerConfigDir := filepath.Join(tempDir, ".docker")
	if err := os.Mkdir(dockerConfigDir, 0o700); err != nil {
		return cleanup, errors.Join(fmt.Errorf("creating Cosign registry config failed: %w", err), cleanup())
	}
	if err := os.WriteFile(filepath.Join(dockerConfigDir, "config.json"), configData, 0o600); err != nil {
		return cleanup, errors.Join(fmt.Errorf("writing Cosign registry config failed: %w", err), cleanup())
	}

	cosignCmd.Env = replaceCommandEnv(cosignCmd.Env, "DOCKER_CONFIG", dockerConfigDir)
	cosignCmd.Env = removeCommandEnv(cosignCmd.Env, "DOCKER_AUTH_CONFIG")
	return cleanup, nil
}

// replaceCommandEnv replaces one variable in a subprocess environment.
func replaceCommandEnv(env []string, key string, value string) []string {
	return append(removeCommandEnv(env, key), key+"="+value)
}

// removeCommandEnv removes one variable from a subprocess environment.
func removeCommandEnv(env []string, key string) []string {
	result := make([]string, 0, len(env))
	for _, entry := range env {
		n, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(n, key) {
			result = append(result, entry)
		}
	}
	return result
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
