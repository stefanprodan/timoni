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
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
)

// LocalFormat identifies an OCI filesystem output format.
type LocalFormat string

const (
	FormatArchive LocalFormat = "oci-archive"
	FormatLayout  LocalFormat = "oci-layout"
)

// localReferenceName matches Timoni's local reference-name syntax.
var localReferenceName = regexp.MustCompile(`^[A-Za-z0-9]+(?:(?:[-._:@/+]|--)[A-Za-z0-9]+)*$`)

// Validate reports whether the local OCI output format is supported.
func (f LocalFormat) Validate() error {
	if f != FormatLayout && f != FormatArchive {
		return fmt.Errorf("unsupported OCI output format %q", f)
	}
	return nil
}

// WriteImage validates and writes an image to a new OCI layout or archive.
// References become OCI layout descriptor names and must use Timoni's local syntax.
func WriteImage(image gcrv1.Image, destination string, format LocalFormat, references []string) error {
	if err := format.Validate(); err != nil {
		return err
	}
	for _, ref := range references {
		if strings.TrimSpace(ref) == "" {
			return fmt.Errorf("OCI reference cannot be empty")
		}
		if !localReferenceName.MatchString(ref) {
			return fmt.Errorf("invalid local OCI reference %q", ref)
		}
	}

	if format == FormatLayout {
		return writeLayout(destination, image, references)
	}

	// Archives are deterministic tar encodings of a temporary OCI layout.
	tmpDir, err := os.MkdirTemp("", "timoni-layout-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	layoutPath := filepath.Join(tmpDir, "layout")
	if err := writeLayout(layoutPath, image, references); err != nil {
		return err
	}
	return writeArchive(layoutPath, destination)
}

// writeLayout creates a layout and removes it if writing fails.
func writeLayout(destination string, image gcrv1.Image, references []string) (err error) {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("creating output parent: %w", err)
	}
	if err := os.Mkdir(destination, 0o755); err != nil {
		return fmt.Errorf("creating output layout: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(destination)
		}
	}()

	path, err := layout.Write(destination, empty.Index)
	if err != nil {
		return err
	}
	if len(references) == 0 {
		return path.AppendImage(image)
	}
	for _, ref := range references {
		if err := path.AppendImage(image, layout.WithAnnotations(map[string]string{
			"org.opencontainers.image.ref.name": ref,
		})); err != nil {
			return err
		}
	}
	return nil
}

// writeArchive creates parent directories, writes a deterministic archive, and removes it if writing fails.
func writeArchive(layoutPath, destination string) (err error) {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("creating output parent: %w", err)
	}
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("creating output archive: %w", err)
	}
	defer func() {
		if closeErr := output.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(destination)
		}
	}()

	tw := tar.NewWriter(output)
	defer func() {
		if closeErr := tw.Close(); err == nil {
			err = closeErr
		}
	}()

	var entries []string
	if err := filepath.Walk(layoutPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path != layoutPath {
			entries = append(entries, path)
		}
		return nil
	}); err != nil {
		return err
	}
	// Stable path order and normalized headers make archive bytes reproducible.
	sort.Strings(entries)

	for _, path := range entries {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() && !info.IsDir() {
			return fmt.Errorf("unsupported layout entry %q", path)
		}

		name, err := filepath.Rel(layoutPath, path)
		if err != nil {
			return err
		}
		header := &tar.Header{
			Name:     filepath.ToSlash(name),
			Mode:     0o644,
			Size:     info.Size(),
			ModTime:  time.Unix(0, 0).UTC(),
			Typeflag: tar.TypeReg,
			Format:   tar.FormatPAX,
		}
		if info.IsDir() {
			header.Name += "/"
			header.Mode = 0o755
			header.Size = 0
			header.Typeflag = tar.TypeDir
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(tw, file)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
	}
	return nil
}
