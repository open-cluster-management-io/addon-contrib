package helpers

import "os"

const (
	AddOnName                    = "clusternet"
	DefaultInstallationNamespace = "open-cluster-management-agent-addon"
)

// GetCurrentNamespace returns the current namespace from file system, if the namespace is not found, it returns the defaultNamespace
func GetCurrentNamespace(defaultNamespace string) string {
	nsBytes, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return defaultNamespace
	}
	return string(nsBytes)
}
