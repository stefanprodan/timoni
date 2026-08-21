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

package oci

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
)

func TestProcessCosignIOReturnsExitError(t *testing.T) {
	g := NewWithT(t)
	cmd := cosignHelperCommand(t, "exit-error")

	err := processCosignIO(context.Background(), logr.Discard(), cmd)

	g.Expect(err).To(MatchError(ContainSubstring("exit status 23")))
	g.Expect(cmd.ProcessState).ToNot(BeNil())
}

func TestNewCosignCommandRegistryFlags(t *testing.T) {
	for _, operation := range []string{"sign", "verify"} {
		t.Run(operation, func(t *testing.T) {
			g := NewWithT(t)

			secure := newCosignCommand(context.Background(), "cosign", operation, false)
			insecure := newCosignCommand(context.Background(), "cosign", operation, true)

			g.Expect(secure.Args).To(Equal([]string{"cosign", operation}))
			g.Expect(insecure.Args).To(Equal([]string{
				"cosign", operation, "--allow-http-registry", "--allow-insecure-registry",
			}))
		})
	}
}

func TestConfigureCosignCredentials(t *testing.T) {
	for name, tt := range map[string]struct {
		credentials string
		imageRef    string
		authKey     string
		expected    dockerAuthConfig
	}{
		"username and password": {
			credentials: "timoni:secret",
			imageRef:    "registry.example.com/org/app@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			authKey:     "registry.example.com",
			expected: dockerAuthConfig{
				Auth: base64.StdEncoding.EncodeToString([]byte("timoni:secret")),
			},
		},
		"registry token": {
			credentials: "secret-token",
			imageRef:    "registry.example.com/org/app:1.0.0",
			authKey:     "registry.example.com",
			expected: dockerAuthConfig{
				RegistryToken: "secret-token",
			},
		},
		"Docker Hub": {
			credentials: "timoni:secret",
			imageRef:    "docker.io/org/app:1.0.0",
			authKey:     "https://index.docker.io/v1/",
			expected: dockerAuthConfig{
				Auth: base64.StdEncoding.EncodeToString([]byte("timoni:secret")),
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)
			sourceConfigDir := t.TempDir()
			existingConfig := `{"auths":{"registry.example.com":{"auth":"stale"}},"credsStore":"desktop","credHelpers":{"registry.example.com":"helper"}}`
			g.Expect(os.WriteFile(filepath.Join(sourceConfigDir, "config.json"), []byte(existingConfig), 0o600)).To(Succeed())
			cmd := exec.Command("cosign", "sign")
			cmd.Env = append(os.Environ(), "HOME=/original/home", "DOCKER_CONFIG="+sourceConfigDir, `docker_auth_config={"auths":{}}`)

			cleanup, err := configureCosignCredentials(cmd, tt.imageRef, tt.credentials)
			g.Expect(err).NotTo(HaveOccurred())
			defer cleanup()

			dockerConfigDir := commandEnv(cmd, "DOCKER_CONFIG")
			g.Expect(commandEnv(cmd, "HOME")).To(Equal("/original/home"))
			g.Expect(dockerConfigDir).NotTo(Equal(sourceConfigDir))
			g.Expect(commandEnv(cmd, "DOCKER_AUTH_CONFIG")).To(BeEmpty())
			g.Expect(strings.Join(cmd.Args, " ")).NotTo(ContainSubstring("secret"))

			configPath := filepath.Join(dockerConfigDir, "config.json")
			configData, err := os.ReadFile(configPath)
			g.Expect(err).NotTo(HaveOccurred())
			var config dockerConfig
			g.Expect(json.Unmarshal(configData, &config)).To(Succeed())
			var rawConfig map[string]json.RawMessage
			g.Expect(json.Unmarshal(configData, &rawConfig)).To(Succeed())
			g.Expect(rawConfig).To(HaveLen(1))
			g.Expect(config.Auths).To(Equal(map[string]dockerAuthConfig{
				tt.authKey: tt.expected,
			}))

			if runtime.GOOS != "windows" {
				info, err := os.Stat(configPath)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))
			}

			g.Expect(cleanup()).To(Succeed())
			_, err = os.Stat(filepath.Dir(dockerConfigDir))
			g.Expect(err).To(MatchError(os.ErrNotExist))
		})
	}
}

func TestConfigureCosignCredentialsSkipsEmptyCredentials(t *testing.T) {
	g := NewWithT(t)
	cmd := exec.Command("cosign", "verify")
	cmd.Env = os.Environ()

	cleanup, err := configureCosignCredentials(cmd, "registry.example.com/org/app:1.0.0", "")

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cleanup()).To(Succeed())
	g.Expect(commandEnv(cmd, "HOME")).To(Equal(os.Getenv("HOME")))
}

func commandEnv(cmd *exec.Cmd, key string) string {
	for _, v := range slices.Backward(cmd.Env) {
		name, value, _ := strings.Cut(v, "=")
		if strings.EqualFold(name, key) {
			return value
		}
	}
	return ""
}

func TestProcessCosignIODrainsOutputConcurrently(t *testing.T) {
	g := NewWithT(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := cosignHelperCommandContext(t, ctx, "large-stderr")

	err := processCosignIO(ctx, logr.Discard(), cmd)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cmd.ProcessState).ToNot(BeNil())
	g.Expect(cmd.ProcessState.Success()).To(BeTrue())
}

func TestProcessCosignIOAcceptsLargeCosignJSONOutput(t *testing.T) {
	g := NewWithT(t)
	cmd := cosignHelperCommand(t, "long-line")

	err := processCosignIO(context.Background(), logr.Discard(), cmd)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cmd.ProcessState).ToNot(BeNil())
	g.Expect(cmd.ProcessState.Success()).To(BeTrue())
}

func TestScanCosignOutputAcceptsLineAtLogLimit(t *testing.T) {
	g := NewWithT(t)

	g.Expect(scanCosignOutput(logr.Discard(), strings.NewReader(strings.Repeat("x", maxCosignOutputLogLine)))).To(Succeed())
}

func TestProcessCosignIODoesNotFailOnOutputLoggingError(t *testing.T) {
	g := NewWithT(t)
	cmd := cosignHelperCommand(t, "overlong-line")

	err := processCosignIO(context.Background(), logr.Discard(), cmd)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cmd.ProcessState).ToNot(BeNil())
	g.Expect(cmd.ProcessState.Success()).To(BeTrue())
}

func TestScanCosignOutputIgnoresClosedPipe(t *testing.T) {
	g := NewWithT(t)
	output, input, err := os.Pipe()
	g.Expect(err).NotTo(HaveOccurred())
	defer input.Close()
	g.Expect(output.Close()).To(Succeed())

	g.Expect(scanCosignOutput(logr.Discard(), output)).To(Succeed())
}

func TestProcessCosignIOReturnsExitErrorDespiteOutputLoggingError(t *testing.T) {
	g := NewWithT(t)
	cmd := cosignHelperCommand(t, "long-line-exit-error")

	err := processCosignIO(context.Background(), logr.Discard(), cmd)

	g.Expect(err).To(MatchError(And(
		ContainSubstring("exit status 23"),
		Not(ContainSubstring("token too long")),
	)))
}

func TestProcessCosignIOHonorsContext(t *testing.T) {
	g := NewWithT(t)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	cmd := cosignHelperCommandContext(t, ctx, "wait")

	err := processCosignIO(ctx, logr.Discard(), cmd)

	g.Expect(err).To(MatchError(ContainSubstring(context.DeadlineExceeded.Error())))
	g.Expect(cmd.ProcessState).ToNot(BeNil())
}

func TestProcessCosignIOClosesInheritedPipes(t *testing.T) {
	g := NewWithT(t)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	cmd := cosignHelperCommandContext(t, ctx, "grandchild")
	start := time.Now()

	err := processCosignIO(ctx, logr.Discard(), cmd)

	g.Expect(err).To(MatchError(ContainSubstring(context.DeadlineExceeded.Error())))
	g.Expect(time.Since(start)).To(BeNumerically("<", 1500*time.Millisecond))
	g.Expect(cmd.ProcessState).ToNot(BeNil())
}

func cosignHelperCommand(t *testing.T, mode string) *exec.Cmd {
	t.Helper()
	return cosignHelperCommandContext(t, context.Background(), mode)
}

func cosignHelperCommandContext(t *testing.T, ctx context.Context, mode string) *exec.Cmd {
	t.Helper()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestCosignHelperProcess", "--", mode)
	cmd.Env = append(os.Environ(), "GO_WANT_COSIGN_HELPER_PROCESS=1")
	return cmd
}

func TestCosignHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_COSIGN_HELPER_PROCESS") != "1" {
		return
	}
	mode := os.Args[len(os.Args)-1]
	switch mode {
	case "exit-error":
		os.Exit(23)
	case "large-stderr":
		_, _ = fmt.Fprint(os.Stderr, strings.Repeat("stderr\n", 32*1024))
	case "long-line":
		_, _ = fmt.Fprint(os.Stdout, strings.Repeat("x", 128*1024))
	case "long-line-exit-error":
		_, _ = fmt.Fprint(os.Stdout, strings.Repeat("x", 128*1024))
		os.Exit(23)
	case "overlong-line":
		_, _ = fmt.Fprint(os.Stdout, strings.Repeat("x", 2*1024*1024))
	case "wait":
		time.Sleep(time.Hour)
	case "grandchild":
		cmd := exec.Command(os.Args[0], "-test.run=TestCosignHelperProcess", "--", "hold-pipes")
		cmd.Env = append(os.Environ(), "GO_WANT_COSIGN_HELPER_PROCESS=1")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			os.Exit(25)
		}
		time.Sleep(time.Hour)
	case "hold-pipes":
		time.Sleep(2 * time.Second)
	default:
		os.Exit(24)
	}
}
