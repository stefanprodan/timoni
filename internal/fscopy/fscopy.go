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

// Package fscopy implements portable filesystem copy operations
// for Timoni modules stored on the local filesystem.
package fscopy

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

var (
	// ErrSymlinkCycle is returned when following symlinks
	// leads back to a directory currently being copied.
	ErrSymlinkCycle = errors.New("symlink cycle detected")

	// ErrTooManyEntries is returned when a copy exceeds
	// the maximum number of filesystem entries.
	ErrTooManyEntries = errors.New("too many filesystem entries")
)

// defaultMaxEntries caps a single copy operation to guard against
// runaway trees, e.g. symlink diamonds expanding exponentially.
const defaultMaxEntries = 1 << 20

// Options configures the CopyDir operation.
type Options struct {
	// FollowSymlinks resolves symbolic links and copies their targets
	// as regular files and directories. Cycles are detected and reported
	// as ErrSymlinkCycle, and dangling links fail the copy.
	// When false (the default), symlinks are skipped.
	FollowSymlinks bool

	// Skip is called before copying each entry with the source path and
	// whether the entry is a directory. Returning true excludes the entry,
	// and for directories, everything under it, without reading it.
	// The path is always the logical location under the source root, even
	// for content materialized from symlink targets living elsewhere.
	// A symlink is first reported with isDir set to false; when following
	// symlinks and the target is a directory, Skip is consulted again with
	// isDir set to true before the directory is copied.
	Skip func(src string, isDir bool) bool

	// MaxEntries caps the total number of copied entries.
	// Zero applies a default of about one million.
	MaxEntries int
}

// CopyDir recursively copies the contents of srcDir into dstDir,
// creating dstDir if needed. Permission bits are preserved, while
// setuid, setgid and sticky bits are deliberately dropped, and
// timestamps are not kept. Entries that are neither regular files,
// directories, nor symlinks (sockets, pipes, devices) are skipped.
// Symlink handling is controlled by opts.FollowSymlinks.
// All writes are confined to dstDir: symlinks already present under
// the destination cannot redirect them elsewhere.
func CopyDir(srcDir, dstDir string, opts Options) error {
	srcAbs, err := filepath.Abs(srcDir)
	if err != nil {
		return err
	}
	dstAbs, err := filepath.Abs(dstDir)
	if err != nil {
		return err
	}

	// Resolve the source root when following symlinks so that every
	// directory visited during the walk is identified by its real path,
	// which the cycle detection in copyDirEntry relies on.
	if opts.FollowSymlinks {
		if srcAbs, err = filepath.EvalSymlinks(srcAbs); err != nil {
			return fmt.Errorf("resolving source %q: %w", srcDir, err)
		}
	}

	srcInfo, err := os.Stat(srcAbs)
	if err != nil {
		return err
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("source %q is not a directory", srcDir)
	}

	// Guard against copying the source into itself using fully resolved
	// paths, as the destination or one of its existing ancestors may be
	// a symlink pointing inside the source.
	srcReal, err := filepath.EvalSymlinks(srcAbs)
	if err != nil {
		return err
	}
	dstReal, err := resolveExistingPrefix(dstAbs)
	if err != nil {
		return err
	}
	if dstReal == srcReal || isPathUnder(dstReal, srcReal) {
		return fmt.Errorf("destination %q is inside source %q", dstDir, srcDir)
	}

	c := &copier{
		opts:       opts,
		srcRoot:    srcAbs,
		maxEntries: opts.MaxEntries,
		visiting:   make(map[string]struct{}),
	}
	if c.maxEntries <= 0 {
		c.maxEntries = defaultMaxEntries
	}

	if err := os.MkdirAll(dstAbs, 0o755); err != nil {
		return err
	}

	// Perform all destination writes through an os.Root so that symlinks
	// under the destination cannot redirect them outside of it.
	c.dst, err = os.OpenRoot(dstAbs)
	if err != nil {
		return err
	}
	defer c.dst.Close()

	if opts.FollowSymlinks {
		c.visiting[srcAbs] = struct{}{}
	}
	if err := c.copyTree(srcAbs, "."); err != nil {
		return err
	}
	return os.Chmod(dstAbs, srcInfo.Mode().Perm())
}

// CopyFile copies the regular file src to dst, creating parent
// directories as needed and preserving the permission bits.
// Symlinks are resolved, and copying a file onto itself,
// also through hard links or symlinks, is rejected.
func CopyFile(src, dst string) (err error) {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !srcInfo.Mode().IsRegular() {
		return fmt.Errorf("source %q is not a regular file", src)
	}
	if dstInfo, err := os.Stat(dst); err == nil && os.SameFile(srcInfo, dstInfo) {
		return fmt.Errorf("source and destination %q are the same file", src)
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode().Perm())
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	// The OpenFile mode is subject to the process umask,
	// restore the source permission bits explicitly.
	if err := out.Chmod(srcInfo.Mode().Perm()); err != nil {
		return err
	}

	_, err = io.Copy(out, in)
	return err
}

// copier tracks the state of a single CopyDir operation.
// The source tree is read at its physical (symlink-resolved) locations,
// while destination paths and the paths reported to Skip are relative
// to the logical roots, keeping ignore semantics and layout stable.
type copier struct {
	opts       Options
	srcRoot    string
	dst        *os.Root
	entries    int
	maxEntries int

	// visiting holds the resolved paths of the directories on the
	// current recursion path, used to detect symlink cycles.
	// Directories are removed on exit so that multiple links to the
	// same target are allowed, while true cycles are not.
	visiting map[string]struct{}
}

// copyTree copies the entries of the physical directory physDir into
// the destination directory at rel (a path relative to the roots).
func (c *copier) copyTree(physDir, rel string) error {
	entries, err := os.ReadDir(physDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		childRel := filepath.Join(rel, entry.Name())
		logical := filepath.Join(c.srcRoot, childRel)
		childPhys := filepath.Join(physDir, entry.Name())
		isLink := entry.Type()&os.ModeSymlink != 0

		// Consult Skip before any stat so that broken entries
		// under ignored paths cannot fail the copy.
		if c.opts.Skip != nil && c.opts.Skip(logical, entry.IsDir()) {
			continue
		}
		if isLink && !c.opts.FollowSymlinks {
			continue
		}
		if err := c.count(logical); err != nil {
			return err
		}

		switch {
		case isLink:
			if err := c.copySymlink(childPhys, childRel, logical); err != nil {
				return err
			}
		case entry.IsDir():
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if err := c.copyDirEntry(childPhys, childRel, info.Mode().Perm()); err != nil {
				return err
			}
		default:
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.Mode().IsRegular() {
				if err := c.copyFile(childPhys, childRel, info.Mode().Perm()); err != nil {
					return err
				}
			}
			// Sockets, pipes and devices are intentionally not copied.
		}
	}

	return nil
}

// copyDirEntry copies the physical directory physDir to rel, deferring
// the permission change until after the recursion so that read-only
// source directories do not prevent their own contents from being
// written. When following symlinks, physDir is a fully resolved path
// and is tracked on the recursion path so that a link back to any
// ancestor is caught as a cycle.
func (c *copier) copyDirEntry(physDir, rel string, perm os.FileMode) error {
	if c.opts.FollowSymlinks {
		if _, ok := c.visiting[physDir]; ok {
			return fmt.Errorf("%w: %q is already being copied", ErrSymlinkCycle, physDir)
		}
		c.visiting[physDir] = struct{}{}
		defer delete(c.visiting, physDir)
	}

	if err := c.dst.Mkdir(rel, 0o755); err != nil {
		if info, serr := c.dst.Stat(rel); serr != nil || !info.IsDir() {
			return err
		}
	}
	if err := c.copyTree(physDir, rel); err != nil {
		return err
	}
	return c.dst.Chmod(rel, perm)
}

// copySymlink materializes the target of the symlink at physPath into
// the destination at rel. The logical path is reported to Skip when the
// target turns out to be a directory, applying directory ignore rules
// to the materialized form.
func (c *copier) copySymlink(physPath, rel, logical string) error {
	realPath, err := filepath.EvalSymlinks(physPath)
	if err != nil {
		return fmt.Errorf("resolving symlink %q: %w", physPath, err)
	}
	realInfo, err := os.Stat(realPath)
	if err != nil {
		return err
	}

	switch {
	case realInfo.IsDir():
		if c.opts.Skip != nil && c.opts.Skip(logical, true) {
			return nil
		}
		return c.copyDirEntry(realPath, rel, realInfo.Mode().Perm())
	case realInfo.Mode().IsRegular():
		return c.copyFile(realPath, rel, realInfo.Mode().Perm())
	default:
		// The link target is a socket, pipe or device, skip it
		// like their non-symlinked counterparts.
		return nil
	}
}

// copyFile copies the regular file at physPath into the destination
// at rel through the destination root, rejecting copies onto the
// source file itself.
func (c *copier) copyFile(physPath, rel string, perm os.FileMode) error {
	srcInfo, err := os.Stat(physPath)
	if err != nil {
		return err
	}
	if dstInfo, err := c.dst.Stat(rel); err == nil && os.SameFile(srcInfo, dstInfo) {
		return fmt.Errorf("source and destination %q are the same file", physPath)
	}

	in, err := os.Open(physPath)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := c.dst.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	// The OpenFile mode is subject to the process umask,
	// restore the source permission bits explicitly.
	if err := out.Chmod(perm); err != nil {
		return err
	}

	_, err = io.Copy(out, in)
	return err
}

// count enforces the entry budget across the whole copy operation.
func (c *copier) count(srcPath string) error {
	c.entries++
	if c.entries > c.maxEntries {
		return fmt.Errorf("%w: copying %q exceeds the limit of %d entries",
			ErrTooManyEntries, srcPath, c.maxEntries)
	}
	return nil
}

// resolveExistingPrefix resolves symlinks in the longest existing
// prefix of path, rejoining the non-existing remainder unchanged.
func resolveExistingPrefix(path string) (string, error) {
	suffix := ""
	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			return filepath.Join(resolved, suffix), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", err
		}
		suffix = filepath.Join(filepath.Base(path), suffix)
		path = parent
	}
}

// isPathUnder reports whether path is lexically inside dir.
func isPathUnder(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return filepath.IsLocal(rel)
}
