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

// Package mask hides the values of Kubernetes Secrets in the objects
// that Timoni prints, so that rendered manifests can be shared without
// exposing credentials.
package mask

import (
	ssautil "github.com/fluxcd/pkg/ssa/utils"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Value replaces the values of the Secret data and stringData entries.
const Value = "***"

// lastAppliedAnnotation holds a copy of the object set by kubectl apply.
const lastAppliedAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

// secretFields are the Secret fields whose values are masked.
var secretFields = []string{"data", "stringData"}

// SecretData returns a copy of the object with every entry under the
// Secret data and stringData fields replaced by the mask, along with the
// kubectl last-applied-configuration annotation. A data or stringData
// field that is not a map is replaced by the mask as a whole. Objects
// that are not Secrets are returned unchanged.
func SecretData(obj *unstructured.Unstructured) *unstructured.Unstructured {
	if obj == nil || !ssautil.IsSecret(obj) {
		return obj
	}

	masked := obj.DeepCopy()
	for _, field := range secretFields {
		raw, found := masked.Object[field]
		if !found {
			continue
		}
		entries, isMap := raw.(map[string]any)
		if !isMap {
			masked.Object[field] = Value
			continue
		}
		for key := range entries {
			entries[key] = Value
		}
	}
	if annotations := masked.GetAnnotations(); annotations != nil {
		if _, found := annotations[lastAppliedAnnotation]; found {
			annotations[lastAppliedAnnotation] = Value
			masked.SetAnnotations(annotations)
		}
	}
	return masked
}
