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

// This file defines the DynamicScorer custom resource definition (CRD) for Kubernetes.
// DynamicScorer CRs are used to register scoring APIs and to configure how scores are fetched.

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"open-cluster-management.io/dynamic-scoring/pkg/common"
)

type AuthConfig struct {
	// TokenSecretRef specifies the reference to the Kubernetes Secret that contains the authentication token.
	TokenSecretRef SecretRef `json:"tokenSecretRef"`
}

type SecretRef struct {
	// The name of the Kubernetes Secret that contains the authentication token.
	Name string `json:"name"`
	// The key within the Kubernetes Secret that contains the authentication token.
	Key  string `json:"key"`
}

type SourceConfigWithAuth struct {
	// +kubebuilder:validation:Enum=Prometheus;None
	// Type specifies the source type (e.g., Prometheus, None).
	Type   common.SourceType    `json:"type,omitempty"`
	// Host specifies the host used by the Dynamic Scoring Agent to retrieve data from the source.
	// For example, if the agent retrieves data from https://your-prometheus.local/api/v1/query_range,
	// https://your-prometheus.local should be specified.
	// This field is ignored when `Type` is `None`.
	Host   string               `json:"host,omitempty"`
	// Path specifies the path used by the Dynamic Scoring Agent to retrieve data from the source.
	// For example, if the agent retrieves data from https://your-prometheus.local/api/v1/query_range,
	// /api/v1/query_range should be specified.
	// This field is ignored when `Type` is `None`.
	Path   string               `json:"path,omitempty"`
	// Params specifies the query parameters used by the Dynamic Scoring Agent to query the data source.
	// This field is ignored when `Type` is `None`.
	Params *common.SourceParams `json:"params,omitempty"`
	// Auth specifies the authentication configuration used by the Dynamic Scoring Agent to retrieve the data source.
	// This field is optional and ignored when empty or when `Type` is `None`.
	Auth   *AuthConfig          `json:"auth,omitempty"`
}

type ScoringConfigWithAuth struct {
	// Host specifies the host used by the Dynamic Scoring Agent to call the scoring API.
	// For example, if the agent calls the scoring API at https://your-scorer.local/score,
	// https://your-scorer.local should be specified.
	Host   string                `json:"host,omitempty"`
	// Path specifies the path used by the Dynamic Scoring Agent to call the scoring API.
	// For example, if the agent calls the scoring API at https://your-scorer.local/score,
	// /score should be specified.
	// If not specified, the default value is `/score`.
	Path   string                `json:"path,omitempty"`
	// Params specifies the scoring parameters used by the Dynamic Scoring Agent to call the scoring API.
	// e.g., interval
	Params *common.ScoringParams `json:"params,omitempty"`
	// Auth specifies the authentication configuration used by the Dynamic Scoring Agent to call the scoring API.
	// This field is optional and ignored when empty.
	Auth   *AuthConfig           `json:"auth,omitempty"`
}

// DynamicScorerSpec defines the desired state of DynamicScorer.
type DynamicScorerSpec struct {
	// Description provides a brief explanation of the Dynamic Scorer's purpose.
	Description string `json:"description"`
	// ConfigURL specifies the URL from which the Dynamic Scoring Agent fetches its configuration.
	ConfigURL   string `json:"configURL"`
	// +kubebuilder:validation:Enum=Full;None
	// ConfigSyncMode determines how the Dynamic Scoring Agent synchronizes its configuration.
	// Full mode means the agent fetches the complete configuration from the specified ConfigURL,
	// while None mode indicates that no configuration synchronization is performed.
	ConfigSyncMode common.ConfigSyncMode `json:"configSyncMode"`
	// +kubebuilder:validation:Enum=Internal;External
	// Location indicates whether the Dynamic Scorer is hosted within the cluster (`Internal`)
	// or outside the cluster (`External`).
	Location common.Location       `json:"location,omitempty"`
	// Source defines the data source configuration for the Dynamic Scoring Agent to fetch metrics.
	Source   SourceConfigWithAuth  `json:"source,omitempty"`
	// Scoring defines the scoring API configuration for the Dynamic Scoring Agent to obtain scores.
	Scoring  ScoringConfigWithAuth `json:"scoring,omitempty"`
	// +kubebuilder:validation:Enum=AddOnPlacementScore;None
	// ScoreDestination specifies where the Dynamic Scoring Agent should send the computed scores.
	// For example, `AddOnPlacementScore` indicates the scores should be sent to the AddOnPlacementScore API.
	ScoreDestination     common.ScoreDestination `json:"scoreDestination,omitempty"`
	// ScoreDimensionFormat defines the format of the score dimension used by the Dynamic Scoring Agent.
	// Score dimensions categorize calculated scores by the AddOnPlacementScore value name.
	// Some reserved placeholders are available.
	// Example placeholders: ${cluster}, ${node}, ${device}, ${namespace}, ${app}, ${pod}, ${container}, ${meta}, ${scoreName}
	// Example format: "resource-usage-${cluster}-${namespace}-${app}"
	ScoreDimensionFormat string                  `json:"scoreDimensionFormat,omitempty"`
}

// DynamicScorerStatus defines the observed state of DynamicScorer.
type DynamicScorerStatus struct {
	// +kubebuilder:validation:Enum=Active;Inactive;Unknown
	HealthStatus     common.ScorerHealthStatus `json:"healthStatus"`
	// LastSyncedConfig holds the last configuration successfully synced by the Dynamic Scoring Agent.
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
