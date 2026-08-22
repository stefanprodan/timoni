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

package main

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

func Test_ReadModuleImages(t *testing.T) {
	tests := []struct {
		name   string
		images string
		want   []string
		errMsg string
	}{
		{
			name: "repository tag and digest",
			images: `package main
values: image: {
	repository: *"docker.io/redis" | string
	tag:        *"8.10.1-alpine" | string
	digest:     *"sha256:becdda6c7f4b3fb42e42fd7f120bbf5c54c4caaaf16f26da24e4563d2c1f0576" | string
}
`,
			want: []string{"docker.io/redis:8.10.1-alpine@sha256:becdda6c7f4b3fb42e42fd7f120bbf5c54c4caaaf16f26da24e4563d2c1f0576"},
		},
		{
			name: "sorted and deduplicated across components",
			images: `package main
values: {
	web: image: {
		repository: *"docker.io/nginx" | string
		tag:        *"1-alpine" | string
		digest:     *"" | string
	}
	test: image: {
		repository: *"docker.io/curlimages/curl" | string
		tag:        *"8.21.0" | string
		digest:     *"" | string
	}
	sidecar: image: {
		repository: *"docker.io/nginx" | string
		tag:        *"1-alpine" | string
		digest:     *"" | string
	}
}
`,
			want: []string{"docker.io/curlimages/curl:8.21.0", "docker.io/nginx:1-alpine"},
		},
		{
			name: "digest only",
			images: `package main
values: image: {
	repository: *"ghcr.io/org/app" | string
	tag:        *"" | string
	digest:     *"sha256:becdda6c7f4b3fb42e42fd7f120bbf5c54c4caaaf16f26da24e4563d2c1f0576" | string
}
`,
			want: []string{"ghcr.io/org/app@sha256:becdda6c7f4b3fb42e42fd7f120bbf5c54c4caaaf16f26da24e4563d2c1f0576"},
		},
		{
			name: "concrete values",
			images: `package main
values: image: {
	repository: "ghcr.io/org/app"
	tag:        "1.0.0"
}
`,
			want: []string{"ghcr.io/org/app:1.0.0"},
		},
		{
			name: "structs without image fields are skipped",
			images: `package main
values: {
	replicas: 2
	service: port: 80
	image: {
		repository: *"ghcr.io/org/app" | string
		tag:        *"1.0.0" | string
	}
}
`,
			want: []string{"ghcr.io/org/app:1.0.0"},
		},
		{
			name: "no values",
			images: `package main
images: {}
`,
			errMsg: "reading images.cue failed",
		},
		{
			name: "no default",
			images: `package main
values: image: {
	repository: string
	tag:        *"1.0.0" | string
}
`,
			errMsg: "reading images.cue failed at values.image",
		},
		{
			name: "empty repository",
			images: `package main
values: image: {
	repository: *"" | string
	tag:        *"1.0.0" | string
}
`,
			errMsg: "image repository is empty",
		},
		{
			name: "invalid reference",
			images: `package main
values: image: {
	repository: *"ghcr.io/org/app,docker.io/org/app" | string
	tag:        *"1.0.0" | string
}
`,
			errMsg: "invalid image reference",
		},
		{
			name: "invalid tag",
			images: `package main
values: image: {
	repository: *"ghcr.io/org/app" | string
	tag:        *"1.0.0 beta" | string
}
`,
			errMsg: "invalid image reference",
		},
		{
			name: "non-string value",
			images: `package main
values: image: {
	repository: *"ghcr.io/org/app" | string
	tag:        *1 | int
}
`,
			errMsg: "reading images.cue failed at values.image",
		},
		{
			name: "syntax error",
			images: `package main
values: image: {
`,
			errMsg: "reading images.cue failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			modulePath := t.TempDir()
			g.Expect(os.WriteFile(filepath.Join(modulePath, apiv1.ImagesFile), []byte(tt.images), 0o644)).To(Succeed())

			images, err := readModuleImages(modulePath)
			if tt.errMsg != "" {
				g.Expect(err).To(MatchError(ContainSubstring(tt.errMsg)))
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(images).To(Equal(tt.want))
		})
	}
}

func Test_ReadModuleImages_NoFile(t *testing.T) {
	g := NewWithT(t)

	images, err := readModuleImages(t.TempDir())
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(images).To(BeEmpty())
}

func Test_AppendImagesAnnotation(t *testing.T) {
	g := NewWithT(t)
	modulePath := t.TempDir()
	g.Expect(os.WriteFile(filepath.Join(modulePath, apiv1.ImagesFile), []byte(`package main
values: image: {
	repository: *"ghcr.io/org/app" | string
	tag:        *"1.0.0" | string
}
`), 0o644)).To(Succeed())

	// The images are recorded when the annotation is not set.
	annotations := map[string]string{}
	g.Expect(appendImagesAnnotation(modulePath, annotations)).To(Succeed())
	g.Expect(annotations).To(HaveKeyWithValue(apiv1.ImagesAnnotation, "ghcr.io/org/app:1.0.0"))

	// An annotation set by the user is preserved.
	annotations = map[string]string{apiv1.ImagesAnnotation: "docker.io/org/app:2.0.0"}
	g.Expect(appendImagesAnnotation(modulePath, annotations)).To(Succeed())
	g.Expect(annotations).To(HaveKeyWithValue(apiv1.ImagesAnnotation, "docker.io/org/app:2.0.0"))

	// No annotation is set for a module without an images file.
	annotations = map[string]string{}
	g.Expect(appendImagesAnnotation(t.TempDir(), annotations)).To(Succeed())
	g.Expect(annotations).ToNot(HaveKey(apiv1.ImagesAnnotation))
}
