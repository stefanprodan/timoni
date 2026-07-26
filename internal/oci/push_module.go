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
	"fmt"

	"github.com/google/go-containerregistry/pkg/crane"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// PushModule builds and pushes ordered vendor and module layers, then returns
// the module's digest URL.
func PushModule(ociURL, contentPath string, ignorePaths []string, annotations map[string]string, opts []crane.Option) (string, error) {
	ref, err := parseArtifactRef(ociURL)
	if err != nil {
		return "", err
	}

	build, err := BuildModuleImage(contentPath, ignorePaths, annotations)
	if err != nil {
		return "", err
	}
	defer build.Close()

	if err := crane.Push(build.Image, ref.String(), opts...); err != nil {
		return "", fmt.Errorf("pushing artifact failed: %w", err)
	}

	digestURL := ref.Context().Digest(build.Digest.String()).String()
	return fmt.Sprintf("%s%s", apiv1.ArtifactPrefix, digestURL), nil
}
