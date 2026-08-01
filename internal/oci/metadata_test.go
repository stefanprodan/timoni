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
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

func TestParseAnnotationsPreservesEqualsInValue(t *testing.T) {
	g := NewWithT(t)

	annotations, err := ParseAnnotations([]string{"example.com/key=value=with=equals"})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(annotations).To(HaveKeyWithValue("example.com/key", "value=with=equals"))
}

func TestAppendGitMetadataPrecedence(t *testing.T) {
	t.Run("explicit created", func(t *testing.T) {
		g := NewWithT(t)
		t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
		annotations := map[string]string{apiv1.CreatedAnnotation: "2024-01-02T03:04:05Z"}

		AppendGitMetadata(context.Background(), t.TempDir(), annotations)

		g.Expect(annotations).To(HaveKeyWithValue(apiv1.CreatedAnnotation, "2024-01-02T03:04:05Z"))
	})

	t.Run("source date epoch", func(t *testing.T) {
		g := NewWithT(t)
		t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
		annotations := map[string]string{}

		AppendGitMetadata(context.Background(), t.TempDir(), annotations)

		g.Expect(annotations).To(HaveKeyWithValue(apiv1.CreatedAnnotation, "2023-11-14T22:13:20Z"))
	})

	t.Run("git commit", func(t *testing.T) {
		g := NewWithT(t)
		t.Setenv("SOURCE_DATE_EPOCH", "")
		repo := t.TempDir()
		runGitTestCommand(t, repo, "init", "-q")
		g.Expect(os.WriteFile(filepath.Join(repo, "file"), []byte("content"), 0o600)).To(Succeed())
		runGitTestCommand(t, repo, "add", "file")
		cmd := exec.Command("git", "-c", "user.name=Timoni", "-c", "user.email=timoni@example.com", "commit", "-qm", "test")
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE=1700000000 +0000", "GIT_COMMITTER_DATE=1700000000 +0000")
		g.Expect(cmd.Run()).To(Succeed())

		annotations := map[string]string{}
		AppendGitMetadata(context.Background(), repo, annotations)

		g.Expect(annotations).To(HaveKeyWithValue(apiv1.CreatedAnnotation, "2023-11-14T22:13:20Z"))
		g.Expect(annotations[apiv1.RevisionAnnotation]).To(HaveLen(40))
	})

	t.Run("outside git", func(t *testing.T) {
		g := NewWithT(t)
		t.Setenv("SOURCE_DATE_EPOCH", "")
		annotations := map[string]string{}

		AppendGitMetadata(context.Background(), t.TempDir(), annotations)

		g.Expect(annotations).ToNot(HaveKey(apiv1.CreatedAnnotation))
	})
}

func TestAppendGitMetadataHonorsCancellation(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("SOURCE_DATE_EPOCH", "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	annotations := map[string]string{}

	AppendGitMetadata(ctx, t.TempDir(), annotations)

	g.Expect(annotations).To(BeEmpty())
}

func runGitTestCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v: %s", args, err, out)
	}
}
