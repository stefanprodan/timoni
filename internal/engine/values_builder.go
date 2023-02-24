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

	"cuelang.org/go/cue"
)

// ValuesBuilder compiles and merges values files.
type ValuesBuilder struct {
	ctx *cue.Context
}

// NewValuesBuilder creates a ValuesBuilder for the given context.
func NewValuesBuilder(ctx *cue.Context) *ValuesBuilder {
	return &ValuesBuilder{ctx: ctx}
}

// MergeValues merges the given overlays in order using the base as the starting point.
func (b *ValuesBuilder) MergeValues(overlays []string, base string) (cue.Value, error) {
	baseVal, err := ExtractValueFromFile(b.ctx, base, defaultValuesName)
	if err != nil {
		return cue.Value{},
			fmt.Errorf("loading values from %s failed, error: %w", base, err)
	}

	for _, overlay := range overlays {
		overlayVal, err := ExtractValueFromFile(b.ctx, overlay, defaultValuesName)
		if err != nil {
			return cue.Value{},
				fmt.Errorf("loading values from %s failed, error: %w", overlay, err)
		}

		baseVal, err = MergeValue(overlayVal, baseVal)
		if err != nil {
			return cue.Value{},
				fmt.Errorf("merging values from %s failed, error: %w", overlay, err)
		}
	}

	return baseVal, nil
}
