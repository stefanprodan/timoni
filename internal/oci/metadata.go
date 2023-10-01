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
	"strconv"
	"strings"
	"time"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// ParseAnnotations parses the command args in the format key=value
// and returns the OpenContainers annotations.
func ParseAnnotations(args []string) (map[string]string, error) {
	annotations := map[string]string{}
	for _, annotation := range args {
		kv := strings.Split(annotation, "=")
		if len(kv) != 2 {
			return annotations, fmt.Errorf("invalid annotation %s, must be in the format key=value", annotation)
		}
		annotations[kv[0]] = kv[1]
	}

	return annotations, nil
}

// AppendGitMetadata sets the OpenContainers source, revision and created annotations
// from the Git metadata. If the git binary or the .git dir are missing, the created
// date is set to the current UTC date, and the source and revision are not appended.
func AppendGitMetadata(repoPath string, annotations map[string]string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tsCmd := exec.CommandContext(ctx, "git", "--no-pager", "log", "-1", `--format=%ct`)
	tsCmd.Dir = repoPath
	if ts, err := tsCmd.Output(); err == nil && len(ts) > 1 {
		if i, err := strconv.ParseInt(strings.TrimSuffix(string(ts), "\n"), 10, 64); err == nil {
			d := time.Unix(i, 0)
			annotations[apiv1.CreatedAnnotation] = d.Format(time.RFC3339)
		}
	} else {
		ct := time.Now().UTC()
		annotations[apiv1.CreatedAnnotation] = ct.Format(time.RFC3339)
		return
	}

	if _, found := annotations[apiv1.SourceAnnotation]; !found {
		urlCmd := exec.CommandContext(ctx, "git", "config", "--get", "remote.origin.url")
		urlCmd.Dir = repoPath
		if repo, err := urlCmd.Output(); err == nil && len(repo) > 1 {
			annotations[apiv1.SourceAnnotation] = strings.TrimSuffix(string(repo), "\n")
		}
	}

	if _, found := annotations[apiv1.RevisionAnnotation]; !found {
		shaCmd := exec.CommandContext(ctx, "git", "show", "-s", "--format=%H")
		shaCmd.Dir = repoPath
		if commit, err := shaCmd.Output(); err == nil && len(commit) > 1 {
			annotations[apiv1.RevisionAnnotation] = strings.TrimSuffix(string(commit), "\n")
		}
	}
}
