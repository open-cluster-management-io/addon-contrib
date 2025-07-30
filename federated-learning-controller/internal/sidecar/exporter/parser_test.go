package exporter

import (
	"reflect"
	"testing"
)

func TestParseContent(t *testing.T) {
	testCases := []struct {
		name     string
		input    []byte
		expected map[string]float64
	}{
		{
			name:  "valid json with float values",
			input: []byte(`{"metric1": 1.23, "metric2": 4.56}`),
			expected: map[string]float64{
				"metric1": 1.23,
				"metric2": 4.56,
			},
		},
		{
			name:  "valid json with string-represented float values",
			input: []byte(`{"metric1": "7.89", "metric2": "10.11"}`),
			expected: map[string]float64{
				"metric1": 7.89,
				"metric2": 10.11,
			},
		},
		{
			name:  "valid json with mixed float and string-represented float values",
			input: []byte(`{"metric1": 12.13, "metric2": "14.15"}`),
			expected: map[string]float64{
				"metric1": 12.13,
				"metric2": 14.15,
			},
		},
		{
			name:     "empty json",
			input:    []byte(`{}`),
			expected: map[string]float64{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := ParseContetnt(tc.input)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("expected %v, but got %v", tc.expected, actual)
			}
		})
	}
}
