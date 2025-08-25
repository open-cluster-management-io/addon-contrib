package manifests

type FederatedLearningServerParams struct {
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

type FederatedLearningClientParams struct {
	ManifestName       string
	ManifestNamespace  string
	ClientJobNamespace string
	ClientJobName      string
	ClientJobImage     string
	ClientDataConfig   string
	ServerAddress      string
	ObsSidecarImage    string
}
