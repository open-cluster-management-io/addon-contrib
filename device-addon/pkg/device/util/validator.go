package util

import (
	"math"

	"github.com/spf13/cast"
)

func checkValueInRange(valueType string, reading interface{}) bool {
	isValid := false

	if valueType == ValueTypeString || valueType == ValueTypeBool || valueType == ValueTypeObject {
		return true
	}

	if valueType == ValueTypeInt8 || valueType == ValueTypeInt16 ||
		valueType == ValueTypeInt32 || valueType == ValueTypeInt64 {
		val := cast.ToInt64(reading)
		isValid = checkIntValueRange(valueType, val)
	}

	if valueType == ValueTypeUint8 || valueType == ValueTypeUint16 ||
		valueType == ValueTypeUint32 || valueType == ValueTypeUint64 {
		val := cast.ToUint64(reading)
		isValid = checkUintValueRange(valueType, val)
	}

	if valueType == ValueTypeFloat32 || valueType == ValueTypeFloat64 {
		val := cast.ToFloat64(reading)
		isValid = checkFloatValueRange(valueType, val)
	}

	return isValid
}

func checkUintValueRange(valueType string, val uint64) bool {
	var isValid = false
	switch valueType {
	case ValueTypeUint8:
		if val <= math.MaxUint8 {
			isValid = true
		}
	case ValueTypeUint16:
		if val <= math.MaxUint16 {
			isValid = true
		}
	case ValueTypeUint32:
		if val <= math.MaxUint32 {
			isValid = true
		}
	case ValueTypeUint64:
		maxiMum := uint64(math.MaxUint64)
		if val <= maxiMum {
			isValid = true
		}
	}
	return isValid
}

func checkIntValueRange(valueType string, val int64) bool {
	var isValid = false
	switch valueType {
	case ValueTypeInt8:
		if val >= math.MinInt8 && val <= math.MaxInt8 {
			isValid = true
		}
	case ValueTypeInt16:
		if val >= math.MinInt16 && val <= math.MaxInt16 {
			isValid = true
		}
	case ValueTypeInt32:
		if val >= math.MinInt32 && val <= math.MaxInt32 {
			isValid = true
		}
	case ValueTypeInt64:
		isValid = true
	}
	return isValid
}

func checkFloatValueRange(valueType string, val float64) bool {
	var isValid = false
	switch valueType {
	case ValueTypeFloat32:
		if !math.IsNaN(val) && math.Abs(val) <= math.MaxFloat32 {
			isValid = true
		}
	case ValueTypeFloat64:
		if !math.IsNaN(val) && !math.IsInf(val, 0) {
			isValid = true
		}
	}
	return isValid
}
