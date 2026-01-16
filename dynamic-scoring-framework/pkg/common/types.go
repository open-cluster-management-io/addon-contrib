package common

// ConfigSyncMode represents the mode of scoring configuration synchronization.
type ConfigSyncMode string

const (
	// Full: All configurations are synchronized.
	ConfigSyncModeFull ConfigSyncMode = "Full"
	// None: No configurations are synchronized; the CR's fields are used instead.
	// When set to `None`, the Dynamic Scoring Controller only checks the health status of Scorers.
	ConfigSyncModeNone ConfigSyncMode = "None"
)

// Location represents the location of the scorer.
type Location string

const (
	// Internal: Scorer is located inside the cluster (hub or managed).
	LocationInternal Location = "Internal"
	// External: Scorer is located outside the cluster (external to both hub and managed).
	LocationExternal Location = "External"
)

// ScoreDestination represents the destination where scores are sent.
type ScoreDestination string

const (
	// AddOnPlacementScore: Scores are sent to the AddOnPlacementScore resource.
	ScoreDestinationAddOnPlacementScore ScoreDestination = "AddOnPlacementScore"
	// None: Scores are not sent to any destination.
	ScoreDestinationNone ScoreDestination = "None"
)

// ScorerHealthStatus represents the health status of a scorer.
type ScorerHealthStatus string

const (
	// Active: Scorer is active and functioning properly.
	ScorerHealthStatusActive ScorerHealthStatus = "Active"
	// Inactive: Scorer is inactive and not currently functioning.
	ScorerHealthStatusInactive ScorerHealthStatus = "Inactive"
	// Unknown: Scorer health status is unknown.
	ScorerHealthStatusUnknown ScorerHealthStatus = "Unknown"
)

// SourceType represents the type of the source.
type SourceType string

const (
	// Prometheus: Source type is Prometheus.
	SourceTypePrometheus SourceType = "Prometheus"
	// None: No source type specified.
	SourceTypeNone SourceType = "None"
)

// Config represents the scoring configuration.
// Scoring APIs must implement a config endpoint that serves this schema.

type Config struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Source      SourceConfig  `json:"source"`
	Scoring     ScoringConfig `json:"scoring"`
}

// SourceConfig represents the source configuration.
// If the `Type` is `Prometheus`, the `Host` should be the Prometheus server URL.
type SourceConfig struct {
	Type SourceType `json:"type,omitempty"`
	// The hostname or IP address of the source.
	// e.g. `http://prometheus-server.monitoring.svc.cluster.local:9090`
	Host string `json:"host,omitempty"`
	// The API path of the source.
	// e.g. `/api/v1/query_range` for Prometheus
	Path   string       `json:"path"`
	Params SourceParams `json:"params"`
}

type SourceParams struct {
	// The Prometheus query string.
	// To query multiple time series, join queries with semicolons.
	Query string `json:"query"`
	// The time range in seconds for the query.
	Range int `json:"range"`
	// The query resolution step width in seconds.
	Step int `json:"step"`
}

type ScoringConfig struct {
	// The hostname or IP address of the scoring endpoint.
	// e.g. `http://scoring-api.scoring.svc.cluster.local:8080`
	Host string `json:"host,omitempty"`
	// The API path of the scoring endpoint.
	// e.g. `/api/v1/score`
	Path   string        `json:"path"`
	Params ScoringParams `json:"params"`
}

type ScoringParams struct {
	// The name of the score.
	Name string `json:"name"`
	// The interval in seconds for scoring.
	// Note: This interval is also used for config synchronization.
	Interval int `json:"interval"`
}

type Mask struct {
	ClusterName string `json:"clusterName"`
	ScoreName   string `json:"scoreName"`
}

// ScorerSummary represents a summary of a scorer configuration.
// The Dynamic Scoring Controller aggregates scorer configurations and generates this summary.
// The list of summaries is provided to managed clusters as ConfigMap data via ManifestWork.

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

// ----------------------------
// Prometheus HTTP API schemas
// ----------------------------

// PrometheusQueryRangeResponse represents the JSON response from the Prometheus HTTP API
// endpoint: GET /api/v1/query_range
// Ref: https://prometheus.io/docs/prometheus/latest/querying/api/#range-queries
//
// Example (simplified):
//
//	{
//	  "status": "success",
//	  "data": {
//	    "resultType": "matrix",
//	    "result": [ {"metric": {...}, "values": [[<ts>, "<val>"], ...]} ]
//	  }
//	}
type PrometheusQueryRangeResponse struct {
	Status string              `json:"status"`
	Data   PrometheusQueryData `json:"data"`
}

type PrometheusQueryData struct {
	// ResultType is typically `matrix` for range queries.
	ResultType string                   `json:"resultType,omitempty"`
	Result     []PrometheusMatrixSeries `json:"result"`
}

// PrometheusMatrixSeries corresponds to one time-series item in a range query response.
type PrometheusMatrixSeries struct {
	Metric map[string]string `json:"metric"`
	// Values is an array of [ <timestamp>, "<sample_value>" ] pairs.
	// Prometheus returns the sample value as a string in JSON.
	// We keep `interface{}` here because the agent currently forwards the raw structure
	// to the scoring API without parsing values.
	Values [][]interface{} `json:"values"`
}

// ----------------------------
// Scoring API schemas
// ----------------------------

// ScoringRequest is the POST body schema used by the agent when calling a scorer.
// When the source query contains multiple time series joined with semicolons,
// the agent flattens the Prometheus matrix result into this structure.
//
// Example:
//
//	{
//	  "data": [ {"metric": {...}, "values": [[<ts>, "<val>"], ...]}, ... ]
//	}
type ScoringRequest struct {
	Data []PrometheusMatrixSeries `json:"data"`
}

// ScoringResponse is the JSON response schema expected from a scorer.
//
// Example:
//
//	{
//	  "results": [
//	    {"metric": {"node":"..."}, "score": 12.3}
//	  ]
//	}
type ScoringResponse struct {
	Results []ScoringResult `json:"results"`
}

// ScoringResult represents an individual scoring result with associated metric labels and score value.
// NOTE: Metric labels are used to construct the agent's metrics endpoint dimension.
// Mapping example:
//
//	metric: {"node":"node1", "score":"my-score"}
//	-> my_score{ds_node="node1"}
type ScoringResult struct {
	Metric map[string]string `json:"metric"`
	// Computed score value.
	// Note: The score value must be a float64.
	// Because `AddOnPlacementScore` uses integer scores, the agent will cast the float64 to an int when sending to APS.
	Score float64 `json:"score"`
}
