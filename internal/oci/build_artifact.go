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
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fluxcd/pkg/sourceignore"
)

/*
Derived work from:
https://github.com/fluxcd/source-controller/blob/53ee3a3db056b4c024d30a735607b3bccbcde54b/internal/controller/storage.go#L387
https://github.com/fluxcd/pkg/blob/4f69e7842ce20a6432a412a03a0cf7ff5d8b87f5/oci/client/build.go#L35
*/

// BuildArtifact creates the destination file and packages
// the given content (excluding symlinks) using tar+gzip compression.
func BuildArtifact(dstFile, contentPath string, ignorePaths []string) error {
	absDir, err := filepath.Abs(contentPath)
	if err != nil {
		return err
	}

	dirStat, err := os.Stat(absDir)
	if os.IsNotExist(err) {
		return fmt.Errorf("invalid source dir path: %s", absDir)
	}

	ignore := strings.Join(ignorePaths, "\n")
	domain := strings.Split(filepath.Clean(absDir), string(filepath.Separator))
	ps := sourceignore.ReadPatterns(strings.NewReader(ignore), domain)
	matcher := sourceignore.NewMatcher(ps)
	filter := func(p string, fi os.FileInfo) bool {
		return matcher.Match(strings.Split(p, string(filepath.Separator)), fi.IsDir())
	}

	tf, err := os.Create(dstFile)
	if err != nil {
		return err
	}

	sz := &writeCounter{}
	mw := io.MultiWriter(tf, sz)

	gw := gzip.NewWriter(mw)
	tw := tar.NewWriter(gw)
	if err := filepath.Walk(absDir, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Ignore anything that is not a file or directories e.g. symlinks
		if m := fi.Mode(); !(m.IsRegular() || m.IsDir()) {
			return nil
		}

		if len(ignorePaths) > 0 && filter(p, fi) {
			return nil
		}

		header, err := tar.FileInfoHeader(fi, p)
		if err != nil {
			return err
		}
		if dirStat.IsDir() {
			// The name needs to be modified to maintain directory structure
			// as tar.FileInfoHeader only has access to the base name of the file.
			// Ref: https://golang.org/src/archive/tar/common.go?#L6264
			//
			// we only want to do this if a directory was passed in
			relFilePath, err := filepath.Rel(absDir, p)
			if err != nil {
				return err
			}
			// Normalize file path so it works on windows
			header.Name = filepath.ToSlash(relFilePath)
		}

		// Remove any environment specific data.
		header.Gid = 0
		header.Uid = 0
		header.Uname = ""
		header.Gname = ""
		header.ModTime = time.Time{}
		header.AccessTime = time.Time{}
		header.ChangeTime = time.Time{}

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if !fi.Mode().IsRegular() {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			f.Close()
			return err
		}
		if _, err := io.Copy(tw, f); err != nil {
			f.Close()
			return err
		}
		return f.Close()
	}); err != nil {
		tw.Close()
		gw.Close()
		tf.Close()
		return err
	}

	if err := tw.Close(); err != nil {
		gw.Close()
		tf.Close()
		return err
	}
	if err := gw.Close(); err != nil {
		tf.Close()
		return err
	}
	if err := tf.Close(); err != nil {
		return err
	}

	return nil
}

type writeCounter struct {
	written int64
}

func (wc *writeCounter) Write(p []byte) (int, error) {
	n := len(p)
	wc.written += int64(n)
	return n, nil
}
