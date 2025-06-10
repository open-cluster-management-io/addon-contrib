/*
Copyright AppsCode Inc. and Contributors.

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
	core "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	ResourceKindFluxCDConfig = "FluxCDConfig"
	ResourceFluxCDConfig     = "fluxcdconfig"
	ResourceFluxCDConfigs    = "fluxcdconfigs"
)

// FluxCDConfigSpec defines the desired state of FluxCDConfig
type FluxCDConfigSpec struct {
	InstallCRDs bool `json:"installCRDs"`
	// +optional
	CRDs CRDsSpec `json:"crds"`
	// +optional
	Multitenancy Multitenancy `json:"multitenancy"`
	// +optional
	ClusterDomain string `json:"clusterDomain"`
	// +optional
	Cli CliSpec `json:"cli"`
	// +optional
	HelmController ControllerSpec `json:"helmController"`
	// +optional
	ImageAutomationController ControllerSpec `json:"imageAutomationController"`
	// +optional
	ImageReflectionController ControllerSpec `json:"imageReflectionController"`
	// +optional
	KustomizeController KustomizeControllerSpec `json:"kustomizeController"`
	// +optional
	NotificationController NotificationControllerSpec `json:"notificationController"`
	// +optional
	SourceController SourceControllerSpec `json:"sourceController"`
	// +optional
	Policies Policies `json:"policies"`
	// +optional
	Rbac Rbac `json:"rbac"`
	// +optional
	LogLevel string `json:"logLevel"`
	// +optional
	WatchAllNamespaces bool `json:"watchAllNamespaces"`
	// +optional
	ImagePullSecrets []core.LocalObjectReference `json:"imagePullSecrets"`
	// +optional
	ExtraObjects []runtime.RawExtension `json:"extraObjects"`
	// +optional
	Prometheus PrometheusSpec `json:"prometheus"`
}

type CRDsSpec struct {
	// +optional
	Annotations map[string]string `json:"annotations"`
}

type Multitenancy struct {
	// +optional
	Enabled bool `json:"enabled"`
	// +optional
	DefaultServiceAccount string `json:"defaultServiceAccount"`
	// +optional
	Privileged bool `json:"privileged"`
}

type CliSpec struct {
	// +optional
	Image string `json:"image"`
	// +optional
	Tag string `json:"tag"`
	// +optional
	NodeSelector map[string]string `json:"nodeSelector"`
	// +optional
	Affinity core.Affinity `json:"affinity"`
	// +optional
	Tolerations []core.Toleration `json:"tolerations"`
	// +optional
	Annotations map[string]string `json:"annotations"`
	// +optional
	ServiceAccount CliServiceAccountSpec `json:"serviceAccount"`
}

type CliServiceAccountSpec struct {
	// +optional
	Automount bool `json:"automount"`
}

type ControllerSpec struct {
	Create bool `json:"create"`
	// +optional
	Image string `json:"image"`
	// +optional
	Tag string `json:"tag"`
	// +optional
	Resources ResourceRequirements `json:"resources"`
	// +optional
	PriorityClassName string `json:"priorityClassName"`
	// +optional
	Annotations map[string]string `json:"annotations"`
	// +optional
	Labels map[string]string `json:"labels"`
	// +optional
	Container ContainerSpec `json:"container"`
	// +optional
	ExtraEnv []core.EnvVar `json:"extraEnv"`
	// +optional
	ServiceAccount ServiceAccountSpec `json:"serviceAccount"`
	// +optional
	//+kubebuilder:validation:Enum=Always;Never;IfNotPresent;""
	ImagePullPolicy core.PullPolicy `json:"imagePullPolicy"`
	// +optional
	NodeSelector map[string]string `json:"nodeSelector"`
	// +optional
	Affinity core.Affinity `json:"affinity"`
	// +optional
	Tolerations []core.Toleration `json:"tolerations"`
}

type KustomizeControllerSpec struct {
	Create bool `json:"create"`
	// +optional
	Image string `json:"image"`
	// +optional
	Tag string `json:"tag"`
	// +optional
	Resources ResourceRequirements `json:"resources"`
	// +optional
	PriorityClassName string `json:"priorityClassName"`
	// +optional
	Annotations map[string]string `json:"annotations"`
	// +optional
	Labels map[string]string `json:"labels"`
	// +optional
	Container ContainerSpec `json:"container"`
	// +optional
	EnvFrom EnvFromSource `json:"envFrom"`
	// +optional
	ExtraEnv []core.EnvVar `json:"extraEnv"`
	// +optional
	ExtraSecretMounts []core.VolumeMount `json:"extraSecretMounts"`
	// +optional
	ServiceAccount ServiceAccountSpec `json:"serviceAccount"`
	// +optional
	//+kubebuilder:validation:Enum=Always;Never;IfNotPresent;""
	ImagePullPolicy core.PullPolicy `json:"imagePullPolicy"`
	// +optional
	Secret SecretSpec `json:"secret"`
	// +optional
	NodeSelector map[string]string `json:"nodeSelector"`
	// +optional
	Affinity core.Affinity `json:"affinity"`
	// +optional
	Tolerations []core.Toleration `json:"tolerations"`
}

type SecretSpec struct {
	Create bool `json:"create"`
	// +optional
	Name string `json:"name"`
	// +optional
	Data map[string]string `json:"data"`
}

type NotificationControllerSpec struct {
	Create bool `json:"create"`
	// +optional
	Image string `json:"image"`
	// +optional
	Tag string `json:"tag"`
	// +optional
	Resources ResourceRequirements `json:"resources"`
	// +optional
	PriorityClassName string `json:"priorityClassName"`
	// +optional
	Annotations map[string]string `json:"annotations"`
	// +optional
	Labels map[string]string `json:"labels"`
	// +optional
	Container ContainerSpec `json:"container"`
	// +optional
	ExtraEnv []core.EnvVar `json:"extraEnv"`
	// +optional
	ServiceAccount ServiceAccountSpec `json:"serviceAccount"`
	// +optional
	//+kubebuilder:validation:Enum=Always;Never;IfNotPresent;""
	ImagePullPolicy core.PullPolicy `json:"imagePullPolicy"`
	// +optional
	Service ServiceSpec `json:"service"`
	// +optional
	WebhookReceiver WebhookReceiverSpec `json:"webhookReceiver"`
	// +optional
	NodeSelector map[string]string `json:"nodeSelector"`
	// +optional
	Affinity core.Affinity `json:"affinity"`
	// +optional
	Tolerations []core.Toleration `json:"tolerations"`
}

type WebhookReceiverSpec struct {
	// +optional
	Service ServiceSpec `json:"service"`
	// +optional
	Ingress IngressSpec `json:"ingress"`
}

type IngressSpec struct {
	Create      bool                    `json:"create"`
	Annotations map[string]string       `json:"annotations"`
	Labels      map[string]string       `json:"labels"`
	Hosts       []IngressRule           `json:"hosts"`
	TLS         []networking.IngressTLS `json:"tls"`
}

type IngressRule struct {
	Host  string            `json:"host"`
	Paths []HTTPIngressPath `json:"paths"`
}

type HTTPIngressPath struct {
	Path     string `json:"path"`
	PathType string `json:"pathType"`
}

type SourceControllerSpec struct {
	Create bool `json:"create"`
	// +optional
	Image string `json:"image"`
	// +optional
	Tag string `json:"tag"`
	// +optional
	Resources ResourceRequirements `json:"resources"`
	// +optional
	PriorityClassName string `json:"priorityClassName"`
	// +optional
	Annotations map[string]string `json:"annotations"`
	// +optional
	Labels map[string]string `json:"labels"`
	// +optional
	Container ContainerSpec `json:"container"`
	// +optional
	ExtraEnv []core.EnvVar `json:"extraEnv"`
	// +optional
	ServiceAccount ServiceAccountSpec `json:"serviceAccount"`
	// +optional
	//+kubebuilder:validation:Enum=Always;Never;IfNotPresent;""
	ImagePullPolicy core.PullPolicy `json:"imagePullPolicy"`
	// +optional
	Service ServiceSpec `json:"service"`
	// +optional
	NodeSelector map[string]string `json:"nodeSelector"`
	// +optional
	Affinity core.Affinity `json:"affinity"`
	// +optional
	Tolerations []core.Toleration `json:"tolerations"`
}

type ServiceSpec struct {
	// +optional
	Labels map[string]string `json:"labels"`
	// +optional
	Annotations map[string]string `json:"annotations"`
}

// ResourceRequirements describes the compute resource requirements.
type ResourceRequirements struct {
	// Limits describes the maximum amount of compute resources allowed.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// +optional
	Limits core.ResourceList `json:"limits"`
	// Requests describes the minimum amount of compute resources required.
	// If Requests is omitted for a container, it defaults to Limits if that is explicitly specified,
	// otherwise to an implementation-defined value.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// +optional
	Requests core.ResourceList `json:"requests"`
}

type EnvFromSource struct {
	// +optional
	Map LocalObjectReference `json:"map"`
	// +optional
	Secret LocalObjectReference `json:"secret"`
}

type LocalObjectReference struct {
	// Name of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	// TODO: Add other useful fields. apiVersion, kind, uid?
	// +optional
	Name string `json:"name"`
}

type ContainerSpec struct {
	// +optional
	AdditionalArgs []string `json:"additionalArgs"`
}

type ServiceAccountSpec struct {
	Create bool `json:"create"`
	// +optional
	Automount bool `json:"automount"`
	// +optional
	Annotations map[string]string `json:"annotations"`
}

type Policies struct {
	Create bool `json:"create"`
}

type Rbac struct {
	Create bool `json:"create"`
	// +optional
	CreateAggregation bool `json:"createAggregation"`
	// +optional
	Annotations map[string]string `json:"annotations"`
	// +optional
	RoleRef RoleRef `json:"roleRef"`
}

type RoleRef struct {
	Name string `json:"name"`
}

type PrometheusSpec struct {
	// +optional
	PodMonitor PodMonitorSpec `json:"podMonitor"`
}

type PodMonitorSpec struct {
	Create bool `json:"create"`
	// +optional
	PodMetricsEndpoints []MetricsEndpoints `json:"podMetricsEndpoints"`
}

type MetricsEndpoints struct {
	// +optional
	Port string `json:"port"`
	// +optional
	Relabelings []Relabeling `json:"relabelings"`
}

type Relabeling struct {
	// +optional
	SourceLabels []string `json:"sourceLabels"`
	// +optional
	Action string `json:"action"`
	// +optional
	Regex string `json:"regex"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// FluxCDConfig is the Schema for the fluxcdconfigs API
type FluxCDConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec FluxCDConfigSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
//+kubebuilder:object:root=true

// FluxCDConfigList contains a list of FluxCDConfig
type FluxCDConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []FluxCDConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FluxCDConfig{}, &FluxCDConfigList{})
}
