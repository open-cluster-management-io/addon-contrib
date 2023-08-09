package util

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cast"
	"gopkg.in/yaml.v2"

	"open-cluster-management-io/addon-contrib/device-addon/pkg/apis/v1alpha1"
)

const castError = "fail to parse %v reading, %v"

func LoadConfig(configFile string, config any) error {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return err
	}

	if err := yaml.Unmarshal(data, config); err != nil {
		return err
	}

	return nil
}

func ToConfigObj(configProperties map[string]interface{}, configObj any) error {
	data, err := json.Marshal(configProperties)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, configObj)
}

func NewResult(resource v1alpha1.DeviceResource, reading interface{}) (*Result, error) {
	var err error
	valueType := resource.Properties.ValueType
	if !checkValueInRange(valueType, reading) {
		return nil, fmt.Errorf("unsupported type %s in device resource", valueType)
	}

	var val interface{}
	switch valueType {
	case ValueTypeBool:
		val, err = cast.ToBoolE(reading)
		if err != nil {
			return nil, fmt.Errorf(castError, resource.Name, err)
		}
	case ValueTypeString:
		val, err = cast.ToStringE(reading)
		if err != nil {
			return nil, fmt.Errorf(castError, resource.Name, err)
		}
	case ValueTypeUint8:
		val, err = cast.ToUint8E(reading)
		if err != nil {
			return nil, fmt.Errorf(castError, resource.Name, err)
		}
	case ValueTypeUint16:
		val, err = cast.ToUint16E(reading)
		if err != nil {
			return nil, fmt.Errorf(castError, resource.Name, err)
		}
	case ValueTypeUint32:
		val, err = cast.ToUint32E(reading)
		if err != nil {
			return nil, fmt.Errorf(castError, resource.Name, err)
		}
	case ValueTypeUint64:
		val, err = cast.ToUint64E(reading)
		if err != nil {
			return nil, fmt.Errorf(castError, resource.Name, err)
		}
	case ValueTypeInt8:
		val, err = cast.ToInt8E(reading)
		if err != nil {
			return nil, fmt.Errorf(castError, resource.Name, err)
		}
	case ValueTypeInt16:
		val, err = cast.ToInt16E(reading)
		if err != nil {
			return nil, fmt.Errorf(castError, resource.Name, err)
		}
	case ValueTypeInt32:
		val, err = cast.ToInt32E(reading)
		if err != nil {
			return nil, fmt.Errorf(castError, resource.Name, err)
		}
	case ValueTypeInt64:
		val, err = cast.ToInt64E(reading)
		if err != nil {
			return nil, fmt.Errorf(castError, resource.Name, err)
		}
	case ValueTypeFloat32:
		val, err = cast.ToFloat32E(reading)
		if err != nil {
			return nil, fmt.Errorf(castError, resource.Name, err)
		}
	case ValueTypeFloat64:
		val, err = cast.ToFloat64E(reading)
		if err != nil {
			return nil, fmt.Errorf(castError, resource.Name, err)
		}
	case ValueTypeObject:
		val = reading
	default:
		return nil, fmt.Errorf("return result fail, none supported value type: %v", valueType)

	}

	return &Result{
		Name:            resource.Name,
		Type:            valueType,
		Value:           val,
		CreateTimestamp: time.Now().UnixNano(),
	}, nil
}

func FindDeviceResource(name string, resources []v1alpha1.DeviceResource) *v1alpha1.DeviceResource {
	for _, res := range resources {
		if res.Name == name {
			return &res
		}
	}

	return nil
}
