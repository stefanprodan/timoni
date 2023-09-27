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

// AppendCreated tries to determine the last Git commit timestamp.
// If the content path has no git history, it sets the created date to UTC now.
func AppendCreated(ctx context.Context, contentPath string, annotations map[string]string) {
	// Try to determine the last Git commit timestamp
	ct := time.Now().UTC()
	created := ct.Format(time.RFC3339)
	gitCmd := exec.CommandContext(ctx, "git", "--no-pager", "log", "-1", `--format=%ct`)
	gitCmd.Dir = contentPath
	if ts, err := gitCmd.Output(); err == nil && len(ts) > 1 {
		if i, err := strconv.ParseInt(strings.TrimSuffix(string(ts), "\n"), 10, 64); err == nil {
			d := time.Unix(i, 0)
			created = d.Format(time.RFC3339)
		}
	}

	annotations[apiv1.CreatedAnnotation] = created
}

// AppendSource adds the source and revision to the OpenContainers annotations.
func AppendSource(url, revision string, annotations map[string]string) {
	if url != "" {
		annotations[apiv1.SourceAnnotation] = url
	}
	if revision != "" {
		annotations[apiv1.RevisionAnnotation] = revision
	}
}
