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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"open-cluster-management.io/dynamic-scoring/pkg/common"
)

type AuthConfig struct {
	TokenSecretRef SecretRef `json:"tokenSecretRef"`
}

type SecretRef struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

type SourceConfigWithAuth struct {
	// +kubebuilder:validation:Enum=Prometheus;None
	Type   common.SourceType    `json:"type,omitempty"`
	Host   string               `json:"host,omitempty"`
	Path   string               `json:"path,omitempty"`
	Params *common.SourceParams `json:"params,omitempty"`
	Auth   *AuthConfig          `json:"auth,omitempty"`
}

type ScoringConfigWithAuth struct {
	Host   string                `json:"host,omitempty"`
	Path   string                `json:"path,omitempty"`
	Params *common.ScoringParams `json:"params,omitempty"`
	Auth   *AuthConfig           `json:"auth,omitempty"`
}

// DynamicScorerSpec defines the desired state of DynamicScorer.
type DynamicScorerSpec struct {
	Description string `json:"description"`
	ConfigURL   string `json:"configURL"`
	// +kubebuilder:validation:Enum=Full;None
	ConfigSyncMode common.ConfigSyncMode `json:"configSyncMode"`
	// +kubebuilder:validation:Enum=Internal;External
	Location common.Location       `json:"location,omitempty"`
	Source   SourceConfigWithAuth  `json:"source,omitempty"`
	Scoring  ScoringConfigWithAuth `json:"scoring,omitempty"`
	// +kubebuilder:validation:Enum=AddOnPlacementScore;None
	ScoreDestination     common.ScoreDestination `json:"scoreDestination,omitempty"`
	ScoreDimensionFormat string                  `json:"scoreDimensionFormat,omitempty"`
}

// DynamicScorerStatus defines the observed state of DynamicScorer.
type DynamicScorerStatus struct {
	// +kubebuilder:validation:Enum=Active;Inactive;Unknown
	HealthStatus     common.ScorerHealthStatus `json:"healthStatus"`
	LastSyncedConfig *common.Config            `json:"lastSyncedConfig,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

// DynamicScorer is the Schema for the dynamicscorers API.
type DynamicScorer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DynamicScorerSpec   `json:"spec,omitempty"`
	Status DynamicScorerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DynamicScorerList contains a list of DynamicScorer.
type DynamicScorerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DynamicScorer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DynamicScorer{}, &DynamicScorerList{})
}
