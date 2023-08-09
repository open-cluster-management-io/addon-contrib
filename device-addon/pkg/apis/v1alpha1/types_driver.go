package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status

// Driver is the schema for the device driver configuration API
type Driver struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	// spec holds the configuration about a device driver configuration.
	// +kubebuilder:validation:Required
	// +required
	Spec DriverSpec `json:"spec"`

	// status holds the state of this configuration.
	// +optional
	Status DriverStatus `json:"status,omitempty"`
}

// DriverList is a list of Driver
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type DriverList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Driver `json:"items"`
}

type DriverSpec struct {
	DriverConfig `json:",inline"`
}

type DriverStatus struct {
	// conditions describe the state of the current device driver.
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

type DriverConfig struct {
	// DriverType represents device driver type
	// +required
	DriverType string `yaml:"type" json:"type"`

	// Properties represents device driver properties
	// +kubebuilder:validation:XPreserveUnknownFields
	Properties Values `yaml:"properties" json:"properties"`
}
