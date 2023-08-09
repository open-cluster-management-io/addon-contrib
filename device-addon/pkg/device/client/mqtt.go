package client

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/eclipse/paho.golang/paho"

	"k8s.io/klog/v2"
)

type debugLogger struct{}

func (l *debugLogger) Println(v ...interface{}) {
	klog.Infoln(v)
}

func (l *debugLogger) Printf(format string, v ...interface{}) {
	klog.Infof(format, v...)
}

type errorLogger struct{}

func (l *errorLogger) Println(v ...interface{}) {
	klog.Errorln(v)
}

func (l *errorLogger) Printf(format string, v ...interface{}) {
	klog.Errorf(format, v...)
}

func ConnectToMQTTBroker(ctx context.Context, brokerInfo *MQTTBrokerInfo, router paho.Router) (*paho.Client, error) {
	var err error
	var conn net.Conn

	username, password, err := getUser()
	if err != nil {
		return nil, err
	}

	for i := 0; i <= brokerInfo.ConnEstablishingRetry; i++ {
		conn, err = net.Dial("tcp", brokerInfo.Host)
		if err != nil {
			if i >= brokerInfo.ConnEstablishingRetry {
				return nil, fmt.Errorf("failed to connect to MQTT broker %s, %v", brokerInfo.Host, err)
			}

			klog.Warningf("Unable to connect to MQTT broker, %s, retrying", brokerInfo.Host)
			time.Sleep(10 * time.Second)
			continue
		}

		break
	}

	client := paho.NewClient(paho.ClientConfig{Conn: conn})

	if router != nil {
		client.Router = router
	}

	client.SetDebugLogger(&debugLogger{})
	client.SetErrorLogger(&errorLogger{})

	cp := &paho.Connect{
		KeepAlive:  brokerInfo.KeepAlive,
		ClientID:   brokerInfo.ClientId,
		CleanStart: true,
	}

	if len(username) != 0 {
		cp.Username = username
		cp.UsernameFlag = true
	}
	if len(password) != 0 {
		cp.Password = []byte(password)
		cp.PasswordFlag = true
	}

	ca, err := client.Connect(ctx, cp)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MQTT broker %s, %v", brokerInfo.Host, err)
	}
	if ca.ReasonCode != 0 {
		return nil, fmt.Errorf("failed to connect to MQTT broker %s, %d - %s",
			brokerInfo.Host, ca.ReasonCode, ca.Properties.ReasonString)
	}

	klog.Infof("MQTT client %s is connected to %s\n", brokerInfo.ClientId, brokerInfo.Host)
	return client, nil
}

func getUser() (username, password string, err error) {
	return "", "", nil
}
