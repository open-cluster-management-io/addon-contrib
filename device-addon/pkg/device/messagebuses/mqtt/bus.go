package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/eclipse/paho.golang/paho"
	mochi "github.com/mochi-co/mqtt/v2"
	"github.com/mochi-co/mqtt/v2/hooks/auth"
	"github.com/mochi-co/mqtt/v2/listeners"
	"github.com/rs/zerolog"

	"k8s.io/klog/v2"

	"open-cluster-management-io/addon-contrib/device-addon/pkg/apis/v1alpha1"
	"open-cluster-management-io/addon-contrib/device-addon/pkg/device/client"
	"open-cluster-management-io/addon-contrib/device-addon/pkg/device/util"
)

const (
	brokerhost    = "host"
	dataTopic     = "dataTopic"
	payloadFormat = "payloadFormat"
)

const (
	jsonObj = "jsonObj"
	jsonMap = "jsonMap"
)

type payloadFunc func(util.Result) []byte

type MQTTMsgBus struct {
	mqttBroker *mochi.Server
	host       string
	pubClient  *paho.Client
	dataTopic  string
	payload    payloadFunc
}

func NewMQTTMsgBus(config v1alpha1.MessageBusConfig) *MQTTMsgBus {
	m := &MQTTMsgBus{}

	host, ok := config.Properties.Data[brokerhost]
	if !ok {
		klog.Infof("Using build-in MQTT broker as the default broker")
		server := mochi.New(nil)
		l := server.Log.Level(zerolog.ErrorLevel)
		server.Log = &l

		// Allow all connections.
		_ = server.AddHook(new(auth.AllowHook), nil)

		m.mqttBroker = server
		m.host = "127.0.0.1:1883"
	} else {
		m.host = fmt.Sprintf("%s", host)
	}

	ptopic, ok := config.Properties.Data[dataTopic]
	if !ok {
		klog.Infof("Using %s as the default publish topic devices/+/data/+", ptopic)
		ptopic = "devices/+/data/+"
	}
	m.dataTopic = strings.Replace(fmt.Sprintf("%s", ptopic), "+", "%s", -1)

	format, ok := config.Properties.Data[payloadFormat]
	if !ok {
		klog.Infof("Using %s as the default payload format", jsonMap)
		format = jsonMap
	}

	format = fmt.Sprintf("%s", format)
	switch format {
	case jsonObj:
		m.payload = toJsonObj
	case jsonMap:
		m.payload = toJsonMap
	}

	return m
}

func (m *MQTTMsgBus) Start(ctx context.Context) error {
	if m.mqttBroker != nil {
		go func() {
			tcp := listeners.NewTCP("mqttmsgbus", ":1883", nil)
			if err := m.mqttBroker.AddListener(tcp); err != nil {
				klog.Fatal(err)
			}

			if err := m.mqttBroker.Serve(); err != nil {
				klog.Fatal(err)
			}

			klog.Infof("MQTT message bus is started on the localhost")
		}()

		// TODO need a notify mechanism
		time.Sleep(5 * time.Second)
	}

	client, err := client.ConnectToMQTTBroker(
		ctx,
		&client.MQTTBrokerInfo{
			Host:      m.host,
			ClientId:  "msgbus-mqtt-pub-client",
			KeepAlive: 3600,
		},
		nil,
	)
	if err != nil {
		return err
	}

	m.pubClient = client

	klog.Infof("Connect to localhost MQTT message bus")
	return nil
}

func (m *MQTTMsgBus) ReceiveData(deviceName string, result util.Result) error {
	topic := fmt.Sprintf(m.dataTopic, deviceName, result.Name)
	data := m.payload(result)

	klog.Infof("Send data to MQTT message bus, [%s] [%s] %s", topic, deviceName, string(data))
	_, err := m.pubClient.Publish(context.TODO(), &paho.Publish{
		Topic:   topic,
		QoS:     0,
		Payload: data,
	})
	if err != nil {
		// TODO handle this error
		klog.Errorf("failed to send data, %v", err)
		return nil
	}

	return nil
}

func (m *MQTTMsgBus) SendData() error {
	// TODO subscribe to message bus to get the command to send the command to driver
	return nil
}

func (m *MQTTMsgBus) Stop(ctx context.Context) {
	m.pubClient.Disconnect(&paho.Disconnect{ReasonCode: 0})
	m.mqttBroker.Close()
}

func toJsonObj(result util.Result) []byte {
	payload, _ := json.Marshal(result)
	return payload
}

func toJsonMap(result util.Result) []byte {
	payload, _ := json.Marshal(map[string]any{result.Name: result.Value})
	return payload
}
