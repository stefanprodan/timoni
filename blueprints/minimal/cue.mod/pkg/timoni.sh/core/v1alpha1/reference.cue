// Copyright 2023 Stefan Prodan
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import "strings"

// LocalObjectReference is a reference to a Kubernetes object
// inside the same namespace.
#LocalObjectReference: {
	// API version of the referent.
	apiVersion?: string & strings.MaxRunes(256)

	// Kind of the referent.
	kind?: string & strings.MaxRunes(256)

	// Name of the referent.
	name!: string & strings.MaxRunes(256)
}
