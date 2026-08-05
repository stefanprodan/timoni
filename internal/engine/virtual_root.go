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
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
)

// NewVirtualRoot returns an OS-native absolute path that does not exist
// on the host filesystem and is never created, for use as the root
// directory of CUE loads backed entirely by load.Config.Overlay.
// Overlay entries shadow the host filesystem but paths without an entry
// fall through to it, so the root must be guaranteed absent from disk.
func NewVirtualRoot() string {
	base := os.TempDir()
	if abs, err := filepath.Abs(base); err == nil {
		base = abs
	}
	for {
		root := filepath.Join(base, fmt.Sprintf("timoni-vfs-%016x", rand.Uint64()))
		// Accept the candidate on any Lstat error: a path that cannot be
		// inspected cannot be read by the loader fallthrough either.
		if _, err := os.Lstat(root); err != nil {
			return root
		}
	}
}
