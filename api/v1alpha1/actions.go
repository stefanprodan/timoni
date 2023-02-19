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

package v1alpha1

import "fmt"

const (
	EnabledValue  = "enabled"
	DisabledValue = "disabled"
)

var (
	// PruneAction is the annotation that defines if a Kubernetes resource should be garbage collected.
	PruneAction = fmt.Sprintf("action.%s/prune", GroupVersion.Group)

	// ForceAction is the annotation that defines if a Kubernetes resource should be recreated.
	ForceAction = fmt.Sprintf("action.%s/force", GroupVersion.Group)
)
