// Copyright 2023 Stefan Prodan
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import "strings"

// Selector defines the schema for Kubernetes Pod label selector used in Deployments, Services, Jobs, etc.
#Selector: {
	// Name must be unique within a namespace. Is required when creating resources.
	// Name is primarily intended for creation idempotence and configuration definition.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names#names
	#Name!: #InstanceName

	// Map of string keys and values that can be used to organize and categorize (scope and select) objects.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels
	labels: #Labels

	// Standard Kubernetes label: app name.
	labels: "\(#StdLabelName)": #Name
}

// MatchLabelsComponent generates the Kubernetes Selector object for use in resources that manage Pods
#MatchLabelsComponent: {
	// Map of string keys and values that can be used to organize and categorize (scope and select) objects.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels
	#SelectorLabels!: #Labels

	// Component is the name of the component used
	// as a suffix for the generated object name.
	#Component!: string & strings.MaxRunes(30)

	// Map of string keys and values that can be used to organize and categorize (scope and select) objects.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels
	matchLabels: #Labels

	// Add the labels supplied by the generated Selector
	matchLabels: #SelectorLabels

	// Add the standard component label
	matchLabels: "\(#StdLabelComponent)": #Component
}
