package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status

// DeviceAddOnConfig is the schema for the device addon configuration API
type DeviceAddOnConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	// spec holds the configuration about a device addon.
	// +kubebuilder:validation:Required
	// +required
	Spec DeviceAddOnConfigSpec `json:"spec"`

	// status holds the state of this configuration.
	// +optional
	Status DeviceAddOnConfigSpecStatus `json:"status,omitempty"`
}

// DeviceAddOnConfig is a list of DeviceAddOnConfig
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type DeviceAddOnConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []DeviceAddOnConfig `json:"items"`
}

type DeviceAddOnConfigSpec struct {
	MessageBuses []MessageBusConfig `yaml:"messageBuses" json:"messageBuses"`
}

type DeviceAddOnConfigSpecStatus struct {
	// conditions describe the state of the current configuration.
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

type MessageBusConfig struct {
	Enabled        bool   `yaml:"enabled" json:"enabled"`
	MessageBusType string `yaml:"type" json:"type"`
	Properties     Values `yaml:"properties" json:"properties"`
}
