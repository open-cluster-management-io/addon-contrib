package common

type ConfigSyncMode string

const (
	ConfigSyncModeFull ConfigSyncMode = "Full"
	ConfigSyncModeNone ConfigSyncMode = "None"
)

type Location string

const (
	LocationInternal Location = "Internal"
	LocationExternal Location = "External"
)

type ScoreDestination string

const (
	ScoreDestinationAddonPlacementScore ScoreDestination = "AddonPlacementScore"
	ScoreDestinationNone                ScoreDestination = "None"
)

type ScorerHealthStatus string

const (
	ScorerHealthStatusActive   ScorerHealthStatus = "Active"
	ScorerHealthStatusInactive ScorerHealthStatus = "Inactive"
	ScorerHealthStatusUnknown  ScorerHealthStatus = "Unknown"
)

type SourceType string

const (
	SourceTypePrometheus SourceType = "Prometheus"
	SourceTypeNone       SourceType = "None"
)

type Config struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Source      SourceConfig  `json:"source"`
	Scoring     ScoringConfig `json:"scoring"`
}

type SourceConfig struct {
	Type   SourceType   `json:"type,omitempty"`
	Host   string       `json:"host,omitempty"`
	Path   string       `json:"path"`
	Params SourceParams `json:"params"`
}

type SourceParams struct {
	Query string `json:"query"`
	Range int    `json:"range"`
	Step  int    `json:"step"`
}

type ScoringConfig struct {
	Host   string        `json:"host,omitempty"`
	Path   string        `json:"path"`
	Params ScoringParams `json:"params"`
}

type ScoringParams struct {
	Name     string `json:"name"`
	Interval int    `json:"interval"`
}

type Mask struct {
	ClusterName string `json:"clusterName"`
	ScoreName   string `json:"scoreName"`
}

type ScorerSummary struct {
	Name                    string           `json:"name"`
	ScoreName               string           `json:"scoreName"`
	SourceType              SourceType       `json:"sourceType"`
	SourceEndpoint          string           `json:"sourceEndpoint"`
	SourceEndpointAuthName  string           `json:"sourceEndpointAuthName"`
	SourceEndpointAuthKey   string           `json:"sourceEndpointAuthKey"`
	SourceQuery             string           `json:"sourceQuery"`
	SourceRange             int              `json:"sourceRange"`
	SourceStep              int              `json:"sourceStep"`
	ScoringEndpoint         string           `json:"scoringEndpoint"`
	ScoringInterval         int              `json:"scoringInterval"`
	ScoringEndpointAuthName string           `json:"scoringEndpointAuthName"`
	ScoringEndpointAuthKey  string           `json:"scoringEndpointAuthKey"`
	Location                Location         `json:"location"`
	ScoreDestination        ScoreDestination `json:"scoreDestination"`
	ScoreDimensionFormat    string           `json:"scoreDimensionFormat"`
}
