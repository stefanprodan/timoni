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

package fetcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/oci"
)

type OCI struct {
	ctx      context.Context
	src      string
	dst      string
	cacheDir string
	version  string
	creds    string
	insecure bool
}

// NewOCI creates an oci Fetcher for the given module.
func NewOCI(ctx context.Context, src, version, dst, cacheDir, creds string, insecure bool) *OCI {
	return &OCI{
		ctx:      ctx,
		src:      src,
		dst:      dst,
		version:  version,
		cacheDir: cacheDir,
		creds:    creds,
		insecure: insecure,
	}
}

func (f *OCI) GetModuleRoot() string {
	return filepath.Join(f.dst, "module")
}

// Fetch copies the module contents to the destination directory.
// The artifact is pulled from the registry and its contents extracted to the destination dir.
func (f *OCI) Fetch() (*apiv1.ModuleReference, error) {
	dstDir := f.GetModuleRoot()

	ociURL := fmt.Sprintf("%s:%s", f.src, f.version)
	if strings.HasPrefix(f.version, "@") {
		ociURL = fmt.Sprintf("%s%s", f.src, f.version)
	}

	if err := os.MkdirAll(dstDir, os.ModePerm); err != nil {
		return nil, err
	}

	opts := oci.Options(f.ctx, f.creds, f.insecure)
	return oci.PullModule(ociURL, dstDir, f.cacheDir, opts)
}
