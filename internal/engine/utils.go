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

package engine

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue"
	"github.com/fluxcd/pkg/sourceignore"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/fscopy"
)

// IsOCIUrl returns true if the given URL is an OCI URL.
func IsOCIUrl(url string) bool {
	return strings.HasPrefix(url, apiv1.ArtifactPrefix)
}

// IsFileURL returns true if the given URL is a file URL.
func IsFileURL(url string) bool {
	return strings.HasPrefix(url, apiv1.LocalPrefix)
}

// GetEnv returns a map of all environment variables.
func GetEnv() map[string]string {
	vars := make(map[string]string)
	for _, e := range os.Environ() {
		if before, after, ok := strings.Cut(e, "="); ok {
			vars[before] = after
		}
	}
	return vars
}

// CopyDir copies the given directory to the destination directory,
// while excluding files that match the timoni.ignore patterns.
// When resolveSymlinks is true, symbolic links are resolved and their
// targets copied in place of the links, otherwise symlinks are skipped.
func CopyDir(srcDir string, dstDir string, resolveSymlinks bool) (err error) {
	// The ignore matcher domain must be built from the same form of the
	// source path that fscopy passes to the Skip callback: absolute, and
	// fully resolved when resolving symlinks.
	srcDir, err = filepath.Abs(srcDir)
	if err != nil {
		return err
	}
	srcDir, err = filepath.EvalSymlinks(srcDir)
	if err != nil {
		return err
	}
	dstDir = filepath.Clean(dstDir)

	domain := strings.Split(srcDir, string(filepath.Separator))
	ps, err := sourceignore.ReadIgnoreFile(filepath.Join(srcDir, apiv1.IgnoreFile), domain)
	if err != nil {
		return err
	}
	matcher := sourceignore.NewMatcher(ps)

	opt := fscopy.Options{
		FollowSymlinks: resolveSymlinks,
		Skip: func(src string, isDir bool) bool {
			return matcher.Match(strings.Split(src, string(filepath.Separator)), isDir)
		},
	}

	return fscopy.CopyDir(srcDir, dstDir, opt)
}

// ReadIgnoreFile returns the module ignore patterns or an input error.
func ReadIgnoreFile(moduleRoot string) ([]string, error) {
	path := filepath.Join(moduleRoot, apiv1.IgnoreFile)
	var ps []string
	if f, err := os.Open(path); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			s := scanner.Text()
			if !strings.HasPrefix(s, "#") && len(strings.TrimSpace(s)) > 0 {
				ps = append(ps, s)
			}
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return ps, nil
}

// ExtractValueFromFile compiles the given file and
// returns the CUE value that matches the given expression.
func ExtractValueFromFile(ctx *cue.Context, filePath, expr string) (cue.Value, error) {
	vData, err := os.ReadFile(filePath)
	if err != nil {
		return cue.Value{}, err
	}
	return ExtractValueFromBytes(ctx, vData, expr)
}

// ExtractValueFromBytes compiles data and returns the value at expr or an error.
func ExtractValueFromBytes(ctx *cue.Context, data []byte, expr string) (cue.Value, error) {
	vObj := ctx.CompileBytes(data)
	if vObj.Err() != nil {
		return cue.Value{}, vObj.Err()
	}

	value := vObj.LookupPath(cue.ParsePath(expr))
	if err := value.Err(); err != nil {
		return cue.Value{}, err
	}

	return value, nil
}

func ExtractStringFromFile(ctx *cue.Context, filePath, exprPath string) (string, error) {
	vData, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	value := ctx.CompileBytes(vData)
	if value.Err() != nil {
		return "", fmt.Errorf("compiling CUE file failed: %w", value.Err())
	}

	expr := value.LookupPath(cue.ParsePath(exprPath))
	if expr.Err() != nil {
		return "", fmt.Errorf("lookup path failed: %w", expr.Err())
	}

	result, err := expr.String()
	if err != nil {
		return "", fmt.Errorf("reading string failed: %w", err)
	}

	return result, nil
}

// MergeValue merges the given overlay on top of the base CUE value.
// New fields from the overlay are added to the base and
// existing fields are overridden with the overlay values.
func MergeValue(overlay, base cue.Value) (cue.Value, error) {
	r, _ := mergeValue(overlay, base)
	return r, nil
}

func mergeValue(overlay, base cue.Value) (cue.Value, bool) {
	switch base.IncompleteKind() {
	case cue.StructKind:
		return mergeStruct(overlay, base)
	case cue.ListKind:
		return mergeList(overlay, base)
	}
	return overlay, true
}

func mergeStruct(overlay, base cue.Value) (cue.Value, bool) {
	out := overlay
	iter, _ := base.Fields(
		cue.Concrete(true),
		cue.Attributes(true),
		cue.Definitions(true),
		cue.Hidden(true),
		cue.Optional(true),
		cue.Docs(true),
	)

	for iter.Next() {
		s := iter.Selector()
		p := cue.MakePath(s)
		r := overlay.LookupPath(p)
		if r.Exists() {
			v, ok := mergeValue(r, iter.Value())
			if ok {
				out = out.FillPath(p, v)
			}
		} else {
			out = out.FillPath(p, iter.Value())
		}
	}

	return out, true
}

func mergeList(overlay, base cue.Value) (cue.Value, bool) {
	ctx := base.Context()

	ri, _ := overlay.List()
	ti, _ := base.List()

	var out []cue.Value
	for ri.Next() && ti.Next() {
		r, ok := mergeValue(ri.Value(), ti.Value())
		if ok {
			out = append(out, r)
		}
	}
	return ctx.NewList(out...), true
}
