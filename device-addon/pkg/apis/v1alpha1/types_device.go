package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status

// Device is the Schema for the devices API
type Device struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	// spec holds the information about a device.
	// +kubebuilder:validation:Required
	// +required
	Spec DeviceSpec `json:"spec"`

	// status holds the state of a device.
	// +optional
	Status DeviceStatus `json:"status,omitempty"`
}

// DeviceList is a list of Device
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type DeviceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Device `json:"items"`
}

type DeviceSpec struct {
	DeviceConfig `json:",inline"`
}

type DeviceStatus struct {
	// conditions describe the state of the current device.
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// 'R' 'W' 'RW' 'WR' are supported
type ReadWrite string

type DeviceConfig struct {
	// Name represents the device name
	// +required
	Name string `yaml:"name" json:"name"`

	// DriverType represents the device driver type
	// +required
	DriverType string `yaml:"driverType" json:"driverType"`

	// Manufacturer represents the device manufacturer
	// +optional
	Manufacturer string `yaml:"manufacturer,omitempty" json:"manufacturer,omitempty"`

	// Model represents the device model
	// +optional
	Model string `yaml:"model,omitempty" json:"model,omitempty"`

	// Description describe the device information
	// +optional
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// ProtocolProperties represents device protocol properties
	// +optional
	// +kubebuilder:validation:XPreserveUnknownFields
	ProtocolProperties Values `yaml:"protocolProperties,omitempty" json:"protocolProperties,omitempty"`

	// Profile represents the device data profile
	// +required
	Profile DeviceProfile `yaml:"profile" json:"profile"`
}

type DeviceProfile struct {
	// DeviceResources represents device supporting resources
	// +optional
	DeviceResources []DeviceResource `yaml:"deviceResources" json:"deviceResources"`

	//DeviceCommands []DeviceCommand `yaml:"deviceCommands" json:"deviceCommands"`
}

type ResourceProperties struct {
	// ReadWrite
	ReadWrite ReadWrite `yaml:"readWrite" json:"readWrite"`

	// ValueType
	ValueType string `yaml:"valueType" json:"valueType"`

	// Units
	// +optional
	Units string `yaml:"units,omitempty" json:"units,omitempty"`

	// DefaultValue
	// +optional
	DefaultValue string `yaml:"defaultValue,omitempty" json:"defaultValue,omitempty"`

	// Minimum
	// +optional
	Assertion string `yaml:"assertion,omitempty" json:"assertion,omitempty"`

	// Minimum
	// +optional
	MediaType string `yaml:"mediaType,omitempty" json:"mediaType,omitempty"`

	// Minimum
	// +optional
	Minimum *float64 `yaml:"minimum,omitempty" json:"minimum,omitempty"`

	// Maximum
	// +optional
	Maximum *float64 `yaml:"maximum,omitempty" json:"maximum,omitempty"`

	// Mask
	// +optional
	Mask *uint64 `yaml:"mask,omitempty" json:"mask,omitempty"`

	// Shift
	// +optional
	Shift *int64 `yaml:"shift,omitempty" json:"shift,omitempty"`

	// Scale
	// +optional
	Scale *float64 `yaml:"scale,omitempty" json:"scale,omitempty"`

	// Offset
	// +optional
	Offset *float64 `yaml:"offset,omitempty" json:"offset,omitempty"`

	// Base
	// +optional
	Base *float64 `yaml:"base,omitempty" json:"base,omitempty"`

	// Optional
	// +optional
	// +kubebuilder:validation:XPreserveUnknownFields
	Optional Values `yaml:"optional,omitempty" json:"optional,omitempty"`
}

type DeviceResource struct {
	// Name represents the device resource name
	// +required
	Name string `yaml:"name" json:"name"`

	// Description represents the device resource description
	// +optional
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// Name represents the device resource properties
	// +required
	Properties ResourceProperties `yaml:"properties" json:"properties"`

	// Attributes represents the device resource attributes
	// +optional
	// +kubebuilder:validation:XPreserveUnknownFields
	Attributes Values `yaml:"attributes,omitempty" json:"attributes,omitempty"`
}

// type DeviceCommand struct {
// 	Name      string                  `yaml:"name" json:"name"`
// 	ReadWrite ReadWrite               `yaml:"readWrite" json:"readWrite"`
// 	Resources []DeviceCommandResource `yaml:"resources" json:"resources"`
// }

// type DeviceCommandResource struct {
// 	DeviceResource string `yaml:"deviceResource" json:"deviceResource"`
// 	DefaultValue   string `yaml:"defaultValue" json:"defaultValue"`
// }
