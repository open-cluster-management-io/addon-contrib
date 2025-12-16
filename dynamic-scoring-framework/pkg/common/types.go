package common

type Config struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Source      SourceConfig  `json:"source"`
	Scoring     ScoringConfig `json:"scoring"`
}

type SourceConfig struct {
	Type   string       `json:"type,omitempty"`
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
	Name                    string `json:"name"`
	ScoreName               string `json:"scoreName"`
	SourceType              string `json:"sourceType"`
	SourceEndpoint          string `json:"sourceEndpoint"`
	SourceEndpointAuthName  string `json:"sourceEndpointAuthName"`
	SourceEndpointAuthKey   string `json:"sourceEndpointAuthKey"`
	SourceQuery             string `json:"sourceQuery"`
	SourceRange             int    `json:"sourceRange"`
	SourceStep              int    `json:"sourceStep"`
	ScoringEndpoint         string `json:"scoringEndpoint"`
	ScoringInterval         int    `json:"scoringInterval"`
	ScoringEndpointAuthName string `json:"scoringEndpointAuthName"`
	ScoringEndpointAuthKey  string `json:"scoringEndpointAuthKey"`
	Location                string `json:"location"`
	ScoreDestination        string `json:"scoreDestination"`
	ScoreDimensionFormat    string `json:"scoreDimensionFormat"`
}
