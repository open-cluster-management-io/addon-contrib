package manifests

// FlowerServerParams defines the parameters for a Flower server.
type FlowerServerParams struct {
	Namespace           string
	Name                string
	Image               string
	NumberOfRounds      int
	MinAvailableClients int
	StorageVolumeName   string
	ModelDir            string
	InitModel           string
	ListenerType        string
	ListenerPort        int
	CreateService       bool
	ObsSidecarImage     string
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

// FlowerClientParams defines the parameters for a Flower client.
type FlowerClientParams struct {
	ManifestName       string
	ManifestNamespace  string
	ClientJobNamespace string
	ClientJobName      string
	ClientJobImage     string
	ClientDataConfig   string
	ServerAddress      string
	ObsSidecarImage    string
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
