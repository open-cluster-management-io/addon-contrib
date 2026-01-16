/*
Copyright 2025.

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

// This file defines the DynamicScoringConfig custom resource definition (CRD) for Kubernetes.
// DynamicScoringConfig CRs represent distribution configuration for scoring APIs.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"open-cluster-management.io/dynamic-scoring/pkg/common"
)

// DynamicScoringConfigSpec defines the desired state of DynamicScoringConfig.
type DynamicScoringConfigSpec struct {
	// Masks defines a list of masks used to filter out unnecessary scores for specific clusters.
	// For example, if a mask specifies a cluster name, only scores related to that cluster will be considered.
	Masks []common.Mask `json:"masks,omitempty"`
}

// DynamicScoringConfigStatus defines the observed state of DynamicScoringConfig.
type DynamicScoringConfigStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

// DynamicScoringConfig is the Schema for the dynamicscoringconfigs API.
type DynamicScoringConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DynamicScoringConfigSpec   `json:"spec,omitempty"`
	Status DynamicScoringConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DynamicScoringConfigList contains a list of DynamicScoringConfig.
type DynamicScoringConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DynamicScoringConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DynamicScoringConfig{}, &DynamicScoringConfigList{})
}
