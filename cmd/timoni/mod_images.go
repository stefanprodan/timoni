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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/google/go-containerregistry/pkg/name"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// appendImagesAnnotation records the container images declared in the
// module's images.cue file under the sh.timoni.images annotation. Modules
// without an images.cue file carry no annotation, and an annotation set
// explicitly by the user is preserved.
func appendImagesAnnotation(modulePath string, annotations map[string]string) error {
	if _, ok := annotations[apiv1.ImagesAnnotation]; ok {
		return nil
	}

	images, err := readModuleImages(modulePath)
	if err != nil {
		return err
	}
	if len(images) > 0 {
		annotations[apiv1.ImagesAnnotation] = strings.Join(images, ",")
	}
	return nil
}

// readModuleImages returns the sorted image references found in the
// module's images.cue file, in the repository[:tag][@digest] format.
func readModuleImages(modulePath string) ([]string, error) {
	imagesFile := filepath.Join(modulePath, apiv1.ImagesFile)
	data, err := os.ReadFile(imagesFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	values := cuecontext.New().CompileBytes(data, cue.Filename(imagesFile)).
		LookupPath(cue.ParsePath("values"))
	if values.Err() != nil {
		return nil, fmt.Errorf("reading %s failed: %w", apiv1.ImagesFile, values.Err())
	}

	var images []string
	var walkErr error
	values.Walk(func(v cue.Value) bool {
		if walkErr != nil || v.IncompleteKind() != cue.StructKind {
			return walkErr == nil
		}
		repository, tag, digest := v.LookupPath(cue.ParsePath("repository")),
			v.LookupPath(cue.ParsePath("tag")), v.LookupPath(cue.ParsePath("digest"))
		if !repository.Exists() || (!tag.Exists() && !digest.Exists()) {
			return true
		}
		ref, err := imageReference(repository, tag, digest)
		if err != nil {
			walkErr = fmt.Errorf("reading %s failed at %s: %w", apiv1.ImagesFile, v.Path(), err)
			return false
		}
		images = append(images, ref)
		return false
	}, nil)
	if walkErr != nil {
		return nil, walkErr
	}

	slices.Sort(images)
	return slices.Compact(images), nil
}

// imageReference composes the repository[:tag][@digest] reference
// from the default values of the image fields and validates it
// as an OCI image reference.
func imageReference(repository, tag, digest cue.Value) (string, error) {
	ref, err := defaultString(repository)
	if err != nil {
		return "", err
	}
	if ref == "" {
		return "", fmt.Errorf("image repository is empty")
	}
	if tag.Exists() {
		s, err := defaultString(tag)
		if err != nil {
			return "", err
		}
		if s != "" {
			ref += ":" + s
		}
	}
	if digest.Exists() {
		s, err := defaultString(digest)
		if err != nil {
			return "", err
		}
		if s != "" {
			ref += "@" + s
		}
	}
	if _, err := name.ParseReference(ref); err != nil {
		return "", fmt.Errorf("invalid image reference %q: %w", ref, err)
	}
	return ref, nil
}

// defaultString returns the concrete or default string of a value.
func defaultString(v cue.Value) (string, error) {
	if d, ok := v.Default(); ok {
		v = d
	}
	return v.String()
}
