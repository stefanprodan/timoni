// Copyright 2023 Stefan Prodan
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

// ResourceRequirements describes the compute resource requirements.
// More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/.
#ResourceRequirements: {
	// Limits describes the maximum amount of compute resources allowed.
	limits?: #ResourceRequirement

	// Requests describes the minimum amount of compute resources required.
	// Requests cannot exceed Limits.
	requests?: #ResourceRequirement
}

// ResourceRequirement describes the CPU and Memory resource requirements.
#ResourceRequirement: {
	cpu?:    #CPURequirement
	memory?: #MemoryRequirement
}

#CPURequirement:    string & =~"^[1-9]\\d*m$"
#MemoryRequirement: string & =~"^[1-9]\\d*(Mi|Gi)$"
