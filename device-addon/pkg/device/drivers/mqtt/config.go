package mqtt

import "open-cluster-management-io/addon-contrib/device-addon/pkg/device/client"

type Config struct {
	client.MQTTBrokerInfo `json:"inline"`

	SubTopic string `json:"subTopic"`
	PubTopic string `json:"pubTopic"`
}
