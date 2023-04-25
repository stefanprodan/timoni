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
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sync/errgroup"
)

const maxParallelism = 10

type Copier struct {
	src, absSrc, dst string

	skipDirs map[string]struct{}

	mkdirFuncs, cpfileFuncs map[string]func() error
}

// NewCopier creates a module copier for the given source and destination.
func NewCopier(src, dst string) (*Copier, error) {
	absSrc, err := filepath.Abs(src)
	if err != nil {
		return nil, err
	}

	copier := &Copier{
		src:         filepath.Clean(src),
		absSrc:      absSrc,
		dst:         filepath.Clean(dst),
		skipDirs:    map[string]struct{}{},
		mkdirFuncs:  map[string]func() error{},
		cpfileFuncs: map[string]func() error{},
	}
	return copier, nil
}

func (c *Copier) AddSkipDirs(dirs ...string) (err error) {
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		if filepath.IsAbs(dir) {
			dir, err = filepath.Rel(c.absSrc, dir)
			if err != nil {
				return
			}
		}
		c.skipDirs[dir] = struct{}{}
	}
	return
}

func (c *Copier) addMkdirFunc(path string, mode fs.FileMode) {
	if _, exists := c.mkdirFuncs[path]; !exists {
		c.mkdirFuncs[path] = func() error {
			return os.MkdirAll(path, mode)
		}
	}
}

func (c *Copier) addCopyFileFunc(src, dst string, mode fs.FileMode) {
	if _, exists := c.cpfileFuncs[dst]; !exists {
		c.cpfileFuncs[dst] = func() error {
			return copyFile(src, dst, mode)
		}
	}
}

// Scan collects a list relevant of files and directories to copy,
// it excludes everything that is not a '.cue' file.
func (c *Copier) Scan() error {
	if err := c.scanDir(c.src, c.dst); err != nil {
		return err
	}

	emptyDirs := []string{}
	for dirPath := range c.mkdirFuncs {
		isEmpty := true
		for filePath := range c.cpfileFuncs {
			if strings.HasPrefix(filePath, dirPath) {
				isEmpty = false
				break
			}
		}
		if isEmpty {
			emptyDirs = append(emptyDirs, dirPath)
		}
	}
	for _, dirPath := range emptyDirs {
		delete(c.mkdirFuncs, dirPath)
	}
	return nil
}

// Copy the files and directories collected by Scan.
func (c *Copier) Copy() error {
	mkdirs := errgroup.Group{}
	mkdirs.SetLimit(maxParallelism)
	for _, mkdirFunc := range c.mkdirFuncs {
		mkdirs.Go(mkdirFunc)
	}
	if err := mkdirs.Wait(); err != nil {
		return err
	}
	cpfiles := errgroup.Group{}
	cpfiles.SetLimit(maxParallelism)
	for _, copyFileFunc := range c.cpfileFuncs {
		cpfiles.Go(copyFileFunc)
	}
	return cpfiles.Wait()
}

func (c *Copier) scanDir(src, dst string) error {
	if _, skip := c.skipDirs[src]; skip {
		return nil
	}

	si, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !si.IsDir() {
		return fmt.Errorf("source %s is not a directory", src)
	}

	_, err = os.Stat(dst)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		return fmt.Errorf("destination %s already exists", dst)
	}

	c.addMkdirFunc(dst, si.Mode())

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for i := range entries {
		entry := entries[i]

		src := filepath.Join(src, entry.Name())
		dst := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := c.scanDir(src, dst); err != nil {
				return err
			}
		}

		fi, err := entry.Info()
		if err != nil {
			return err
		}

		if fi.Mode().IsRegular() && filepath.Ext(entry.Name()) == ".cue" {
			c.addCopyFileFunc(src, dst, si.Mode())
		}
	}

	return nil
}

// copyFile copies a file from source to destination
// while preserving permissions.
func copyFile(src, dst string, mode fs.FileMode) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer func() {
		if e := out.Close(); e != nil {
			err = e
		}
	}()

	_, err = io.Copy(out, in)
	if err != nil {
		return
	}

	err = out.Sync()
	if err != nil {
		return
	}

	err = os.Chmod(dst, mode)
	if err != nil {
		return
	}

	return
}
