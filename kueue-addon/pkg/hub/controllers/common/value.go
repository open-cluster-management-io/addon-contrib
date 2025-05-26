package common

import "fmt"

const (
	// MultiKueueResourceName is the name used for MultiKueue-related secrets
	MultiKueueResourceName = "multikueue"

	// KueueNamespace is the namespace where Kueue components are deployed
	KueueNamespace = "kueue-system"

	// AdmissionCheckControllerName is the name of the admission check controller
	AdmissionCheckControllerName = "open-cluster-management.io/placement"
)

// Get MultiKueue Kubeconfig Secret name
func GetMultiKueueSecretName(clusterName string) string {
	return fmt.Sprintf("%s-%s", MultiKueueResourceName, clusterName)
}
