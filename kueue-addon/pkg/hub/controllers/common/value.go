package common

import (
	"fmt"
	"os"
)

const (
	// ClusterProxyURLEnv is the environment variable for cluster proxy URL
	ClusterProxyURLEnv = "CLUSTER_PROXY_URL"
	// KueueNamespaceEnv is the environment variable for kueue installed namespace
	KueueNamespaceEnv = "KUEUE_NAMESPACE"
)

var (
	// MultiKueueResourceName is the name used for MultiKueue-related secrets
	MultiKueueResourceName = "multikueue"

	// KueueNamespace is the namespace where Kueue components are deployed
	KueueNamespace = getKueueNamespace()

	// AdmissionCheckControllerName is the name of the admission check controller
	AdmissionCheckControllerName = "open-cluster-management.io/placement"
)

func getKueueNamespace() string {
	ns := os.Getenv(KueueNamespaceEnv)
	if ns == "" {
		return "kueue-system"
	}
	return ns
}

// Get MultiKueue Kubeconfig Secret name
func GetMultiKueueSecretName(clusterName string) string {
	return fmt.Sprintf("%s-%s", MultiKueueResourceName, clusterName)
}
