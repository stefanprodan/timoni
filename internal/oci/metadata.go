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
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// ParseAnnotations parses key=value pairs and preserves additional equals signs
// in values.
func ParseAnnotations(args []string) (map[string]string, error) {
	annotations := map[string]string{}
	for _, annotation := range args {
		key, value, found := strings.Cut(annotation, "=")
		if !found {
			return annotations, fmt.Errorf("invalid annotation %s, must be in the format key=value", annotation)
		}
		annotations[key] = value
	}

	return annotations, nil
}

// AppendGitMetadata fills missing creation, source, and revision annotations.
// Creation uses SOURCE_DATE_EPOCH before Git commit time; Git failures and
// cancellations are ignored.
func AppendGitMetadata(parent context.Context, repoPath string, annotations map[string]string) {
	if info, err := os.Stat(repoPath); err == nil && !info.IsDir() {
		repoPath = filepath.Dir(repoPath)
	}

	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()

	if _, found := annotations[apiv1.CreatedAnnotation]; !found {
		if epoch := os.Getenv("SOURCE_DATE_EPOCH"); epoch != "" {
			if seconds, err := strconv.ParseInt(epoch, 10, 64); err == nil {
				annotations[apiv1.CreatedAnnotation] = time.Unix(seconds, 0).UTC().Format(time.RFC3339)
			}
		}
		if _, found := annotations[apiv1.CreatedAnnotation]; !found {
			if ts, err := gitOutput(ctx, repoPath, "--no-pager", "log", "-1", "--format=%ct"); err == nil {
				if seconds, err := strconv.ParseInt(ts, 10, 64); err == nil {
					annotations[apiv1.CreatedAnnotation] = time.Unix(seconds, 0).UTC().Format(time.RFC3339)
				}
			}
		}
	}

	if _, found := annotations[apiv1.SourceAnnotation]; !found {
		if repo, err := gitOutput(ctx, repoPath, "config", "--get", "remote.origin.url"); err == nil {
			annotations[apiv1.SourceAnnotation] = repo
		}
	}

	if _, found := annotations[apiv1.RevisionAnnotation]; !found {
		if commit, err := gitOutput(ctx, repoPath, "show", "-s", "--format=%H"); err == nil {
			annotations[apiv1.RevisionAnnotation] = commit
		}
	}
}

// gitOutput runs Git in dir and returns trimmed standard output.
func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}
