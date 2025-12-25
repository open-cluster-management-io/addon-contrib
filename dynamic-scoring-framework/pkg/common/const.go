package common

const (
	DynamicScoringNamespace               = "dynamic-scoring"
	DynamicScoringConfigName              = "dynamic-scoring-config"
	ManifestWorkConfigMapName             = "dynamic-scoring-config-manifestwork"
	DynamicScoreName                      = "dynamic_score"
	DynamicScoreRequestsCounterName       = "dynamic_scoring_requests_total"
	DynamicScoreFetchSecondsCounterName   = "dynamic_scoring_fetch_seconds_total"
	DynamicScorePerformSecondsCounterName = "dynamic_scoring_perform_seconds_total"
	DynamicScoreSendSecondsCounterName    = "dynamic_scoring_send_seconds_total"
	DynamicScoreLabelMaxLength            = 256
	DynamicScoreLabelPrefix               = "ds_"
)
