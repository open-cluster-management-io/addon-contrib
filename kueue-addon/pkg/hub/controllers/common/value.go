package common

import (
	"os"
)

const (
	// KueueNamespaceEnv is the environment variable for kueue installed namespace
	KueueNamespaceEnv = "KUEUE_NAMESPACE"
)

var (
	// MultiKueueResourceName is the name used for MultiKueue-related resources
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
