package exporter

import (
	"reflect"
	"testing"
)

func TestParseContent(t *testing.T) {
	testCases := []struct {
		name            string
		input           []byte
		expectedMetrics map[string]float64
		expectedLabels  map[string]float64
	}{
		{
			name:  "valid json with float values",
			input: []byte(`{"metrics": {"metric1": 1.23, "metric2": 4.56}, "labels": {"label1": 1.0}}`),
			expectedMetrics: map[string]float64{
				"metric1": 1.23,
				"metric2": 4.56,
			},
			expectedLabels: map[string]float64{
				"label1": 1.0,
			},
		},
		{
			name:  "valid json with string-represented float values",
			input: []byte(`{"metrics": {"metric1": "7.89", "metric2": "10.11"}, "labels": {"label1": "2.0"}}`),
			expectedMetrics: map[string]float64{
				"metric1": 7.89,
				"metric2": 10.11,
			},
			expectedLabels: map[string]float64{
				"label1": 2.0,
			},
		},
		{
			name:  "valid json with mixed float and string-represented float values",
			input: []byte(`{"metrics": {"metric1": 12.13, "metric2": "14.15"}, "labels": {"label1": 3.0}}`),
			expectedMetrics: map[string]float64{
				"metric1": 12.13,
				"metric2": 14.15,
			},
			expectedLabels: map[string]float64{
				"label1": 3.0,
			},
		},
		{
			name:            "empty json",
			input:           []byte(`{"metrics": {}, "labels": {}}`),
			expectedMetrics: map[string]float64{},
			expectedLabels:  map[string]float64{},
		},
		{
			name:  "json with only metrics",
			input: []byte(`{"metrics": {"metric1": 1.23}, "labels": {}}`),
			expectedMetrics: map[string]float64{
				"metric1": 1.23,
			},
			expectedLabels: map[string]float64{},
		},
		{
			name:            "json with only labels",
			input:           []byte(`{"labels": {"label1": 1.0}, "metrics": {}}`),
			expectedMetrics: map[string]float64{},
			expectedLabels: map[string]float64{
				"label1": 1.0,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualMetrics, actualLabels, err := ParseContetnt(tc.input)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(actualMetrics, tc.expectedMetrics) {
				t.Errorf("expected metrics %v, but got %v", tc.expectedMetrics, actualMetrics)
			}
			if !reflect.DeepEqual(actualLabels, tc.expectedLabels) {
				t.Errorf("expected labels %v, but got %v", tc.expectedLabels, actualLabels)
			}
		})
	}
}

func TestParseContentError(t *testing.T) {
	testCases := []struct {
		name  string
		input []byte
	}{
		{
			name:  "invalid json",
			input: []byte(`{"metrics": {"metric1": 1.23, "metric2": 4.56`),
		},
		{
			name:  "value is not a number or string-represented number",
			input: []byte(`{"metrics": {"metric1": "abc"}}`),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := ParseContetnt(tc.input)
			if err == nil {
				t.Errorf("expected an error, but got nil")
			}
		})
	}
}
