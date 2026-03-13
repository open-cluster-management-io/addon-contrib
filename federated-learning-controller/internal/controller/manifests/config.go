package manifests

// FlowerServerAppParams defines the parameters for a Flower 1.26.x SuperExec-ServerApp Deployment on hub.
type FlowerServerAppParams struct {
	Namespace        string // FederatedLearning CR namespace (where Deployment is created)
	Name             string // e.g., "<cr-name>-serverapp"
	Image            string // app image with ServerApp code
	SuperLinkAddress string // from spec.server.superlink, e.g. "superlink.flower-system:9091"
}

// OpenFLServerParams defines the parameters for an OpenFL server.
type OpenFLServerParams struct {
	Namespace         string
	Name              string
	Image             string
	NumberOfRounds    int
	StorageVolumeName string
	ListenerType      string
	ListenerIP        string
	ListenerPort      int
	ModelDir          string
	CreateService     bool
	ObsSidecarImage   string
	Collaborators     string
}

// FlowerClientAppParams defines the parameters for a Flower 1.26.x SuperExec-ClientApp ManifestWorkReplicaSet.
type FlowerClientAppParams struct {
	Name             string // e.g., "<cr-name>-clientapp"
	Namespace        string // FederatedLearning CR namespace (where MWRS is created)
	PlacementName    string // Placement name for cluster selection
	Image            string // app image with ClientApp code
	SuperNodeAddress string // from spec.client.supernode, e.g. "flower-supernode.flower-addon:9094"
	ClientNamespace  string // derived from supernode endpoint, e.g. "flower-addon"
}

// OpenFLClientParams defines the parameters for an OpenFL client.
type OpenFLClientParams struct {
	ManifestName       string
	ManifestNamespace  string
	ClientJobNamespace string
	ClientJobName      string
	ClientJobImage     string
	ClientDataPath     string
	ServerIP           string
	ServerPort         uint16
	ModelDir           string
	ObsSidecarImage    string
	ClientName         string
	NumberOfRounds     int
}
