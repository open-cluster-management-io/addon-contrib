package dynamic_scoring

import (
	"os"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

func TestGetDefaultValues_DefaultImage(t *testing.T) {
	t.Setenv("DYNAMIC_SCORING_ADDON_IMAGE", "")

	cluster := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"}}
	addon := &addonapiv1alpha1.ManagedClusterAddOn{ObjectMeta: metav1.ObjectMeta{Name: "dynamic-scoring"}}

	values, err := GetDefaultValues(cluster, addon)
	if err != nil {
		t.Fatalf("GetDefaultValues returned error: %v", err)
	}

	secret, ok := values["KubeConfigSecret"].(string)
	if !ok {
		t.Fatalf("expected KubeConfigSecret string, got %T", values["KubeConfigSecret"])
	}
	if secret != "dynamic-scoring-hub-kubeconfig" {
		t.Fatalf("unexpected KubeConfigSecret: %s", secret)
	}

	clusterName, ok := values["ClusterName"].(string)
	if !ok {
		t.Fatalf("expected ClusterName string, got %T", values["ClusterName"])
	}
	if clusterName != "cluster-a" {
		t.Fatalf("unexpected ClusterName: %s", clusterName)
	}

	image, ok := values["Image"].(string)
	if !ok {
		t.Fatalf("expected Image string, got %T", values["Image"])
	}
	if image != DefaultDynamicScoringAddonImage {
		t.Fatalf("unexpected Image: %s", image)
	}
}

func TestGetDefaultValues_EnvOverride(t *testing.T) {
	t.Setenv("DYNAMIC_SCORING_ADDON_IMAGE", "example.com/custom:latest")

	cluster := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster-b"}}
	addon := &addonapiv1alpha1.ManagedClusterAddOn{ObjectMeta: metav1.ObjectMeta{Name: "addon-b"}}

	values, err := GetDefaultValues(cluster, addon)
	if err != nil {
		t.Fatalf("GetDefaultValues returned error: %v", err)
	}

	image, ok := values["Image"].(string)
	if !ok {
		t.Fatalf("expected Image string, got %T", values["Image"])
	}
	if image != "example.com/custom:latest" {
		t.Fatalf("unexpected Image: %s", image)
	}
}

func TestNewRegistrationOption(t *testing.T) {
	option := NewRegistrationOption(&rest.Config{}, "dynamic-scoring", "agent")
	if option == nil {
		t.Fatalf("expected non-nil registration option")
	}
	if option.CSRConfigurations == nil {
		t.Fatalf("expected CSRConfigurations to be set")
	}
	if option.CSRApproveCheck == nil {
		t.Fatalf("expected CSRApproveCheck to be set")
	}
	if option.PermissionConfig == nil {
		t.Fatalf("expected PermissionConfig to be set")
	}
}

func TestAgentHealthProber(t *testing.T) {
	prober := AgentHealthProber()
	if prober == nil {
		t.Fatalf("expected non-nil health prober")
	}
	if prober.Type != "DeploymentAvailability" {
		t.Fatalf("unexpected prober type: %s", prober.Type)
	}
}

func TestGetDefaultValues_DoesNotMutateEnv(t *testing.T) {
	os.Unsetenv("DYNAMIC_SCORING_ADDON_IMAGE")
	cluster := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster-c"}}
	addon := &addonapiv1alpha1.ManagedClusterAddOn{ObjectMeta: metav1.ObjectMeta{Name: "addon-c"}}

	values, err := GetDefaultValues(cluster, addon)
	if err != nil {
		t.Fatalf("GetDefaultValues returned error: %v", err)
	}

	if _, ok := values["ImagePullSecrets"]; !ok {
		t.Fatalf("expected ImagePullSecrets key to exist")
	}
}
