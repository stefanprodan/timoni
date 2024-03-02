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
	"strings"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

type Fetcher interface {
	Fetch() (*apiv1.ModuleReference, error)
	GetModuleRoot() string
}

type Options struct {
	// Source is the location of the module to fetch.
	Source string
	// Version is the version of the module to fetch.
	Version string
	// Destination is the location to store the fetched module.
	Destination string
	// CacheDir is the location to store the fetched module.
	CacheDir string
	// Creds is the credentials to use when fetching the module.
	Creds string
	// Insecure is a flag that indicates if the fetcher should allow use of insecure connections.
	Insecure bool

	// DefaultLocal is a flag that indicates if the fetcher should default to a Local fetcher if no other can be applied.
	DefaultLocal bool
}

// New is a factory function that creates a new Fetcher based on the provided options.
// If you know the type of fetcher you want to create, prefer using the specific factory function.
func New(ctx context.Context, opts Options) (Fetcher, error) {
	switch {
	case strings.HasPrefix(opts.Source, apiv1.ArtifactPrefix):
		return NewOCI(ctx, opts.Source, opts.Version, opts.Destination, opts.CacheDir, opts.Creds, opts.Insecure), nil
	case strings.HasPrefix(opts.Source, apiv1.LocalPrefix):
		return NewLocal(opts.Source, opts.Destination), nil
	default:
		if opts.DefaultLocal {
			return NewLocal(opts.Source, opts.Destination), nil
		}
		return nil, fmt.Errorf("unsupported module source %s", opts.Source)
	}
}
