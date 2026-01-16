package dynamic_scoring

import (
	"embed"
	"fmt"
	"os"

	"k8s.io/client-go/rest"
	"open-cluster-management.io/addon-framework/pkg/addonfactory"
	"open-cluster-management.io/addon-framework/pkg/agent"
	"open-cluster-management.io/addon-framework/pkg/utils"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"open-cluster-management.io/dynamic-scoring/pkg/rbac"
)

const (
	DefaultDynamicScoringAddonImage = "quay.io/open-cluster-management/dynamic-scoring-addon:latest"
	AddonName                       = "dynamic-scoring"
	InstallationNamespace           = "dynamic-scoring"
)

//go:embed manifests
//go:embed manifests/templates
var FS embed.FS

func NewRegistrationOption(kubeConfig *rest.Config, addonName, agentName string) *agent.RegistrationOption {
	return &agent.RegistrationOption{
		CSRConfigurations: agent.KubeClientSignerConfigurations(addonName, agentName),
		CSRApproveCheck:   utils.DefaultCSRApprover(agentName),
		PermissionConfig:  rbac.AddonRBAC(kubeConfig),
	}
}

func GetDefaultValues(cluster *clusterv1.ManagedCluster,
	addon *addonapiv1alpha1.ManagedClusterAddOn) (addonfactory.Values, error) {

	image := os.Getenv("DYNAMIC_SCORING_ADDON_IMAGE")
	if len(image) == 0 {
		image = DefaultDynamicScoringAddonImage
	}

	manifestConfig := struct {
		KubeConfigSecret string
		ClusterName      string
		Image            string
		ImagePullSecrets []string
	}{
		KubeConfigSecret: fmt.Sprintf("%s-hub-kubeconfig", addon.Name),
		ClusterName:      cluster.Name,
		Image:            image,
		ImagePullSecrets: nil,
	}

	return addonfactory.StructToValues(manifestConfig), nil
}

func AgentHealthProber() *agent.HealthProber {
	return &agent.HealthProber{
		Type: agent.HealthProberTypeDeploymentAvailability,
	}
}
