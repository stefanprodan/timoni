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

package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ModuleSourceDigest returns the SHA-256 digest of a module directory.
// The digest includes sorted relative paths and file contents; symlink target
// strings are included without following links, which keeps the digest deterministic.
func ModuleSourceDigest(root string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("module source %s is not a directory", root)
	}

	type sourceEntry struct {
		path       string
		linkTarget string
	}
	var files []sourceEntry
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			files = append(files, sourceEntry{path: path, linkTarget: target})
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("module source contains non-regular file %s", path)
		}
		files = append(files, sourceEntry{path: path})
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })

	h := sha256.New()
	for _, file := range files {
		rel, err := filepath.Rel(root, file.path)
		if err != nil {
			return "", err
		}
		rel = filepath.ToSlash(rel)
		if _, err := io.WriteString(h, fmt.Sprintf("%d:%s\x00", len(rel), rel)); err != nil {
			return "", err
		}
		if file.linkTarget != "" {
			if _, err := io.WriteString(h, "link:"+file.linkTarget+"\x00"); err != nil {
				return "", err
			}
			continue
		}
		f, err := os.Open(file.path)
		if err != nil {
			return "", err
		}
		_, copyErr := io.Copy(h, f)
		closeErr := f.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func validSHA256Digest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}
