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
	"maps"
	"regexp"
	"slices"
	"strings"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/validate/content"
	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var regexHintRE = regexp.MustCompile(`regex used for validation is '([^']*)'`)

// shortenRule collapses an apimachinery validation message to
// "must match regex '<pattern>'" when the message carries the regex hint.
func shortenRule(msg string) string {
	m := regexHintRE.FindStringSubmatch(msg)
	if len(m) < 2 {
		return msg
	}
	return "must match regex '" + m[1] + "'"
}

// rbacKinds enumerates the kinds which the kube-apiserver validates with
// path-segment rules instead of DNS-1123 subdomain.
var rbacKinds = map[string]struct{}{
	"Role":               {},
	"ClusterRole":        {},
	"RoleBinding":        {},
	"ClusterRoleBinding": {},
}

// isRBACObject reports whether the object belongs to the Kubernetes RBAC
// API group with a kind whose name allows path-segment characters.
func isRBACObject(obj *unstructured.Unstructured) bool {
	if _, ok := rbacKinds[obj.GetKind()]; !ok {
		return false
	}
	return obj.GroupVersionKind().Group == rbacv1.GroupName
}

// validateObjectMeta checks that the object carries the apiVersion, kind and
// metadata.name fields required for it to be applied on the cluster and
// tracked in the instance inventory, then validates the metadata name,
// namespace, labels and annotations with the kube-apiserver ObjectMeta rules.
// It returns one error per violation.
func validateObjectMeta(obj *unstructured.Unstructured) []error {
	var missing []string
	if obj.GetAPIVersion() == "" {
		missing = append(missing, "apiVersion")
	}
	if obj.GetKind() == "" {
		missing = append(missing, "kind")
	}
	if obj.GetName() == "" {
		missing = append(missing, "metadata.name")
	}
	if len(missing) > 0 {
		return []error{fmt.Errorf("missing required field(s) %s", strings.Join(missing, ", "))}
	}

	var errs []error

	nameFn := apivalidation.NameIsDNSSubdomain
	if isRBACObject(obj) {
		nameFn = func(name string, _ bool) []string {
			return content.IsPathSegmentName(name)
		}
	}
	for _, msg := range nameFn(obj.GetName(), false) {
		errs = append(errs, fmt.Errorf("metadata.name %q %s", obj.GetName(), shortenRule(msg)))
	}

	if ns := obj.GetNamespace(); ns != "" {
		for _, msg := range apivalidation.ValidateNamespaceName(ns, false) {
			errs = append(errs, fmt.Errorf("metadata.namespace %q %s", ns, shortenRule(msg)))
		}
	}

	metadata, _ := obj.Object["metadata"].(map[string]any)
	if labels, ok := metadata["labels"].(map[string]any); ok {
		errs = append(errs, validateLabels(labels)...)
	}
	if annotations, ok := metadata["annotations"].(map[string]any); ok {
		errs = append(errs, validateAnnotations(annotations)...)
	}

	return errs
}

// validateLabels validates the label keys and values with the
// kube-apiserver rules, returning one error per violation.
func validateLabels(labels map[string]any) []error {
	var errs []error
	for _, k := range slices.Sorted(maps.Keys(labels)) {
		v, ok := labels[k].(string)
		if !ok {
			errs = append(errs, fmt.Errorf("metadata.labels[%q] must be a string", k))
			continue
		}
		for _, msg := range content.IsLabelKey(k) {
			errs = append(errs, fmt.Errorf("metadata.labels[%q] key %s", k, shortenRule(msg)))
		}
		for _, msg := range content.IsLabelValue(v) {
			errs = append(errs, fmt.Errorf("metadata.labels[%q] value %s", k, shortenRule(msg)))
		}
	}
	return errs
}

// validateAnnotations validates the annotation keys and the total
// annotations size with the kube-apiserver rules, returning one error
// per violation. Annotation keys are case-insensitive for the
// qualified-name check, matching apivalidation.ValidateAnnotations.
func validateAnnotations(annotations map[string]any) []error {
	var errs []error
	var totalSize int64
	for _, k := range slices.Sorted(maps.Keys(annotations)) {
		v, ok := annotations[k].(string)
		if !ok {
			errs = append(errs, fmt.Errorf("metadata.annotations[%q] must be a string", k))
			continue
		}
		for _, msg := range content.IsLabelKey(strings.ToLower(k)) {
			errs = append(errs, fmt.Errorf("metadata.annotations[%q] key %s", k, shortenRule(msg)))
		}
		totalSize += int64(len(k)) + int64(len(v))
	}
	if totalSize > int64(apivalidation.TotalAnnotationSizeLimitB) {
		errs = append(errs, fmt.Errorf("metadata.annotations total size must be at most %d bytes",
			apivalidation.TotalAnnotationSizeLimitB))
	}
	return errs
}
