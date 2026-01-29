package dynamic_scoring_agent

import (
	"strings"
	"testing"

	"open-cluster-management.io/dynamic-scoring/pkg/common"
)

func TestSanitizeResourceName(t *testing.T) {
	name := sanitizeResourceName("My Score@Name")
	if name != "my-score-name" {
		t.Fatalf("unexpected sanitized name: %s", name)
	}

	empty := sanitizeResourceName("")
	if empty != "score" {
		t.Fatalf("unexpected sanitized empty name: %s", empty)
	}
}

func TestLabelKey(t *testing.T) {
	key := labelKey("a", "b", "c")
	if key != "a||b||c" {
		t.Fatalf("unexpected label key: %s", key)
	}
}

func TestAggregateLabels(t *testing.T) {
	out := aggregateLabels([]string{"a", "", "b", ""})
	if out != "a;b" {
		t.Fatalf("unexpected aggregate labels: %s", out)
	}
}

func TestRenderDimansionName(t *testing.T) {
	format := "${cluster}-${scoreName}-${node}"
	mapping := map[string]string{
		"${cluster}":   "cluster-a",
		"${scoreName}": "latency",
		"${node}":      "node-1",
	}

	name := renderDimansionName(format, mapping)
	if name != "cluster-a-latency-node-1" {
		t.Fatalf("unexpected rendered name: %s", name)
	}
}

func TestGetValidString(t *testing.T) {
	metric := map[string]string{"node": "node-1"}
	if got := getValidString(metric, "node", true); got != "node-1" {
		t.Fatalf("unexpected value: %s", got)
	}

	longValue := strings.Repeat("a", common.DynamicScoreLabelMaxLength+10)
	metric["meta"] = longValue
	truncated := getValidString(metric, "meta", true)
	if len(truncated) != common.DynamicScoreLabelMaxLength {
		t.Fatalf("expected truncation to %d, got %d", common.DynamicScoreLabelMaxLength, len(truncated))
	}
}
