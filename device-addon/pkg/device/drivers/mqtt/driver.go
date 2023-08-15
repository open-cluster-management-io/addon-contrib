package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/eclipse/paho.golang/paho"

	"k8s.io/klog/v2"

	"open-cluster-management-io/addon-contrib/device-addon/pkg/apis/v1alpha1"
	"open-cluster-management-io/addon-contrib/device-addon/pkg/device/client"
	"open-cluster-management-io/addon-contrib/device-addon/pkg/device/messagebuses"
	"open-cluster-management-io/addon-contrib/device-addon/pkg/device/util"
)

type MQTTDriver struct {
	config   *Config
	client   *paho.Client
	devices  map[string]v1alpha1.DeviceConfig
	msgBuses []messagebuses.MessageBus
	msgChan  chan *paho.Publish
}

func NewMQTTDriver(driverConfig util.ConfigProperties, msgBuses []messagebuses.MessageBus) *MQTTDriver {
	var mqttBrokerInfo = &Config{}
	if err := util.ToConfigObj(driverConfig, mqttBrokerInfo); err != nil {
		klog.Errorf("failed to parse mqtt driver config, %v", err)
		return nil
	}

	return &MQTTDriver{
		devices:  make(map[string]v1alpha1.DeviceConfig),
		msgBuses: msgBuses,
		config:   mqttBrokerInfo,
	}
}

func (d *MQTTDriver) GetType() string {
	return "mqtt"
}

func (d *MQTTDriver) Start(ctx context.Context) error {
	client, err := client.ConnectToMQTTBroker(
		ctx,
		&d.config.MQTTBrokerInfo,
		paho.NewSingleHandlerRouter(func(m *paho.Publish) { d.msgChan <- m }),
	)
	if err != nil {
		return err
	}

	_, err = client.Subscribe(ctx, &paho.Subscribe{
		Subscriptions: map[string]paho.SubscribeOptions{d.config.SubTopic: {QoS: byte(d.config.Qos)}},
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s, %v", d.config.SubTopic, err)
	}

	klog.Infof("Subscribing to %s", d.config.SubTopic)
	go func() {
		for m := range d.msgChan {
			klog.Infof("Receive message [%s], payload=%s", m.Topic, string(m.Payload))
			incomingTopic := m.Topic
			subscribedTopic := d.config.SubTopic
			subscribedTopic = strings.Replace(subscribedTopic, "#", "", -1)
			deviceName := strings.Replace(incomingTopic, subscribedTopic, "", -1)

			device, ok := d.devices[deviceName]
			if !ok {
				klog.Infof("Ignore the unknown device %s", deviceName)
				return
			}

			data := make(util.Attributes)
			if err := json.Unmarshal(m.Payload, &data); err != nil {
				klog.Errorf("failed to unmarshal incoming data for device %s, %v", deviceName, err)
				return
			}

			for key, val := range data {
				res := util.FindDeviceResource(key, device.Profile.DeviceResources)
				if res == nil {
					klog.Warningf("The device  %s attribute %s  is unsupported", deviceName, key)
					continue
				}

				result, err := util.NewResult(*res, val)
				if err != nil {
					klog.Errorf("The device %s attribute %s  is unsupported, %v", deviceName, key, err)
					continue
				}

				// publish the message to message bus
				for _, msgBus := range d.msgBuses {
					msgBus.ReceiveData(deviceName, *result)
				}
			}
		}
	}()

	d.client = client
	return err
}

func (d *MQTTDriver) Stop(ctx context.Context) {
	klog.Info("driver is stopping, disconnect the MQTT conn")
	d.client.Disconnect(&paho.Disconnect{ReasonCode: 0})
}

func (d *MQTTDriver) AddDevice(device v1alpha1.DeviceConfig) error {
	_, ok := d.devices[device.Name]
	if !ok {
		d.devices[device.Name] = device
	}

	return nil
}

func (d *MQTTDriver) RemoveDevice(deviceName string) error {
	//TODO
	return nil
}

func (d *MQTTDriver) RunCommand(command util.Command) error {
	// TODO
	return nil
}
