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

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// GroupVersion is the group version of Timoni's APIs.
	GroupVersion = schema.GroupVersion{Group: "timoni.sh", Version: "v1alpha1"}

	// InstanceKind is the kind name of the Instance type.
	InstanceKind = "Instance"

	// InstanceStorageType is the name of the Kubernetes
	// Secret type used to store the instance metadata and inventory.
	InstanceStorageType = "timoni.sh/instance"

	// FieldManager is the name of the manager performing Kubernetes patch operations.
	FieldManager = "timoni"
)
