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

// Package schemas embeds the Timoni CUE schemas so that they can be
// consumed by the Go API as the single source of truth, while also
// being published as the importable 'timoni.sh/core/v1alpha1' package.
package schemas

import "embed"

// FS embeds the published Timoni CUE schema tree (all API groups and
// versions under timoni.sh/). Consumers read their own subpath, e.g.
// "timoni.sh/core/v1alpha1/bundle.cue".
//
//go:embed timoni.sh
var FS embed.FS
