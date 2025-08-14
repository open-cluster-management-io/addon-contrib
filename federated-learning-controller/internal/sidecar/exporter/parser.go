package exporter

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
)

type flexibleFloat float64

func (f *flexibleFloat) UnmarshalJSON(data []byte) error {
	var num float64
	if err := json.Unmarshal(data, &num); err == nil {
		*f = flexibleFloat(num)
		return nil
	}

	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return fmt.Errorf("value must be a number or a string representing a number: %w", err)
	}

	parsedNum, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return fmt.Errorf("string value could not be parsed into a float: %w", err)
	}

	*f = flexibleFloat(parsedNum)
	return nil
}

func ParseContetnt(content []byte) (map[string]float64, error) {
	metrics := make(map[string]flexibleFloat)

	err := json.Unmarshal(content, &metrics)
	if err != nil {
		log.Printf("JSON unmarshaling failed: %s", err)
		return nil, err
	}

	result := make(map[string]float64, len(metrics))
	for key, value := range metrics {
		result[key] = float64(value)
	}

	return result, nil
}
