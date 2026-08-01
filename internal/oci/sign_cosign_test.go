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
	"fmt"
	"os"
	"os/exec"
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

func TestProcessCosignIOReturnsScannerError(t *testing.T) {
	g := NewWithT(t)
	cmd := cosignHelperCommand(t, "long-line")

	err := processCosignIO(context.Background(), logr.Discard(), cmd)

	g.Expect(err).To(MatchError(ContainSubstring("token too long")))
	g.Expect(cmd.ProcessState).ToNot(BeNil())
}

func TestScanCosignOutputIgnoresClosedPipe(t *testing.T) {
	g := NewWithT(t)
	output, input, err := os.Pipe()
	g.Expect(err).NotTo(HaveOccurred())
	defer input.Close()
	g.Expect(output.Close()).To(Succeed())

	g.Expect(scanCosignOutput(logr.Discard(), output)).To(Succeed())
}

func TestProcessCosignIOReturnsOutputAndExitErrors(t *testing.T) {
	g := NewWithT(t)
	cmd := cosignHelperCommand(t, "long-line-exit-error")

	err := processCosignIO(context.Background(), logr.Discard(), cmd)

	g.Expect(err).To(MatchError(And(
		ContainSubstring("token too long"),
		ContainSubstring("exit status 23"),
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
