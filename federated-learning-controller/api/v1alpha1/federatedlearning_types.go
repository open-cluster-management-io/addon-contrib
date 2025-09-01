/*
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
	clustersv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
)

func init() {
	SchemeBuilder.Register(&FederatedLearning{}, &FederatedLearningList{})
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=fl
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=".status.phase",description="The current phase of the FederatedLearning process"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// FederatedLearning represents the schema for the federated learning API.
type FederatedLearning struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FederatedLearningSpec   `json:"spec,omitempty"`
	Status FederatedLearningStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FederatedLearningList contains a list of FederatedLearning resources.
type FederatedLearningList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FederatedLearning `json:"items"`
}

// Framework represents the federated learning framework.
type Framework string

const (
	Flower Framework = "flower"
	OpenFL Framework = "openfl"
	Other  Framework = "other"
)

const (
	AnnotationSidecarImage = "federated-learning.io/sidecar-image"
)

// FederatedLearningSpec defines the desired state of FederatedLearning.
type FederatedLearningSpec struct {
	// +kubebuilder:default=other
	Framework Framework  `json:"framework,omitempty"`
	Server    ServerSpec `json:"server,omitempty"`
	Client    ClientSpec `json:"client,omitempty"`
}

// ClientSpec defines the specification for the client in federated learning.
type ClientSpec struct {
	Image     string                        `json:"image,omitempty"`
	Placement clustersv1beta1.PlacementSpec `json:"placement,omitempty"`
}

// ServerSpec defines the specification for the server in federated learning.
type ServerSpec struct {
	Image  string `json:"image,omitempty"`
	Rounds int    `json:"rounds,omitempty"`
	// +kubebuilder:validation:Minimum=1
	MinAvailableClients int              `json:"minAvailableClients,omitempty"`
	Listeners           []ListenerSpec   `json:"listeners,omitempty"`
	Storage             ModelStorageSpec `json:"storage,omitempty"`
}

// ModelStorageSpec defines the storage specification for the model.
type ModelStorageSpec struct {
	Name      string      `json:"name,omitempty"`
	Type      StorageType `json:"type,omitempty"`
	Size      string      `json:"size,omitempty"` // +optional
	ModelPath string      `json:"path,omitempty"` //
}

// StorageType represents the type of storage.
type StorageType string

const (
	PersistentVolumeClaim StorageType = "PersistentVolumeClaim"
	HostPathStorage       StorageType = "HostPath"
)

// ListenerSpec defines the specification for a listener.
type ListenerSpec struct {
	Name string `json:"name,omitempty"`
	// +kubebuilder:default:=8080
	Port int          `json:"port,omitempty"`
	Type ListenerType `json:"type,omitempty"`

	// IP is the optional bind IP for NodePort services.
	// It is only applicable when Type is "NodePort".
	// +optional
	IP string `json:"ip,omitempty"`
}

// FederatedLearningStatus defines the observed state of FederatedLearning.
type FederatedLearningStatus struct {
	// +kubebuilder:validation:Enum=Waiting;Running;Completed;Failed;Start
	// +kubebuilder:default:=Waiting
	Phase     Phase            `json:"phase,omitempty"`
	Message   string           `json:"message,omitempty"`
	Listeners []ListenerStatus `json:"listeners,omitempty"`
	// ServerStatus ServerStatus `json:"serverStatus,omitempty"`
	// ClientStatus ClientStatus `json:"clientStatus,omitempty"`
}

// ClientStatus defines the status of the client in federated learning.
type ClientStatus struct {
	PlacementStatus clustersv1beta1.PlacementStatus `json:"placementStatus,omitempty"`
}

// ServerStatus defines the status of the server in federated learning.
type ServerStatus struct {
	ModelPath string           `json:"modelPath,omitempty"`
	Listeners []ListenerStatus `json:"listeners,omitempty"`
}

// ListenerStatus defines the status of a listener.
type ListenerStatus struct {
	Name    string       `json:"name,omitempty"`
	Type    ListenerType `json:"type,omitempty"`
	Address string       `json:"address,omitempty"`
	Port    int          `json:"port,omitempty"`
}

// Phase represents the phase of the federated learning process.
type Phase string

const (
	PhaseStart     Phase = "Start" // Indicates the manual trigger to initiate the federated learning process
	PhaseWaiting   Phase = "Waiting"
	PhaseRunning   Phase = "Running"
	PhaseCompleted Phase = "Completed"
	PhaseFailed    Phase = "Failed"
)

type ListenerType string

const (
	LoadBalancer ListenerType = "LoadBalancer"
	NodePort     ListenerType = "NodePort"
	Route        ListenerType = "Route"
)
