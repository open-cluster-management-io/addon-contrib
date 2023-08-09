package opcua

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/klog/v2"

	"open-cluster-management-io/addon-contrib/device-addon/pkg/apis/v1alpha1"
	"open-cluster-management-io/addon-contrib/device-addon/pkg/device/messagebuses"
	"open-cluster-management-io/addon-contrib/device-addon/pkg/device/util"
)

type request struct {
	nodeId *ua.NodeID
	handle uint32
	res    v1alpha1.DeviceResource
}

type opcuaDevice struct {
	deviceConfig v1alpha1.DeviceConfig
	requests     []request
	cancelFunc   context.CancelFunc
}

type OPCUADriver struct {
	sync.Mutex
	config   *Config
	msgBuses []messagebuses.MessageBus
	devices  map[string]opcuaDevice
}

func NewOPCUADriver(driverConfig util.ConfigProperties, msgBuses []messagebuses.MessageBus) *OPCUADriver {
	var config = &Config{}
	if err := util.ToConfigObj(driverConfig, config); err != nil {
		klog.Errorf("failed to parse opcua driver config %v", err)
		return nil
	}

	return &OPCUADriver{
		devices:  make(map[string]opcuaDevice),
		msgBuses: msgBuses,
		config:   config,
	}
}

func (d *OPCUADriver) GetType() string {
	return "opcua"
}

func (d *OPCUADriver) Start(ctx context.Context) error {
	//do nothing
	return nil
}

func (d *OPCUADriver) Stop(ctx context.Context) {
	d.Lock()
	defer d.Unlock()

	for _, device := range d.devices {
		device.cancelFunc()
	}
}

func (d *OPCUADriver) AddDevice(config v1alpha1.DeviceConfig) error {
	d.Lock()
	defer d.Unlock()

	requests := []request{}

	last, ok := d.devices[config.Name]
	if ok {
		if equality.Semantic.DeepEqual(last.deviceConfig, config) {
			klog.Infof("The device %s already exists", config.Name)
			return nil
		}

		klog.Infof("Restart the device %s", config.Name)
		requests = append(requests, last.requests...)
		last.cancelFunc()
	}

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		klog.Infof("Start device %s", config.Name)
		if err := d.startSubscription(ctx, config); err != nil {
			klog.Errorf("failed to start device %s, %v", config.Name, err)
			cancel()
		}
		klog.Infof("The device %s is done", config.Name)
	}()

	klog.Infof("The device %s is starting", config.Name)
	d.devices[config.Name] = opcuaDevice{
		deviceConfig: config,
		requests:     requests,
		cancelFunc:   cancel,
	}
	return nil
}

func (d *OPCUADriver) RemoveDevice(deviceName string) error {
	d.Lock()
	defer d.Unlock()

	current, ok := d.devices[deviceName]
	if !ok {
		klog.Infof("The device %s is removed", deviceName)
		return nil
	}

	klog.Infof("Remove the device %s", deviceName)
	current.cancelFunc()
	delete(d.devices, deviceName)
	return nil
}

func (d *OPCUADriver) RunCommand(command util.Command) error {
	//TODO
	return nil
}

func (d *OPCUADriver) startSubscription(ctx context.Context, config v1alpha1.DeviceConfig) error {
	endpoint, err := d.findEndpoint(config)
	if err != nil {
		return err
	}

	endpoints, err := opcua.GetEndpoints(ctx, endpoint)
	if err != nil {
		return err
	}
	ep := opcua.SelectEndpoint(
		endpoints,
		d.config.SecurityPolicy,
		ua.MessageSecurityModeFromString(d.config.SecurityMode),
	)
	if ep == nil {
		return fmt.Errorf("failed to find suitable endpoint")
	}

	opts := []opcua.Option{
		opcua.SecurityPolicy(d.config.SecurityPolicy),
		opcua.SecurityModeString(d.config.SecurityMode),
		opcua.CertificateFile(d.config.CertFile),
		opcua.PrivateKeyFile(d.config.KeyFile),
		opcua.AuthAnonymous(),
		opcua.SecurityFromEndpoint(ep, ua.UserTokenTypeAnonymous),
	}

	client := opcua.NewClient(ep.EndpointURL, opts...)
	if err := client.Connect(ctx); err != nil {
		return err
	}
	defer client.CloseWithContext(ctx)

	klog.Infof("Connected to opcua server %s", endpoint)

	notifyCh := make(chan *opcua.PublishNotificationData)
	interval := time.Duration(500) * time.Millisecond
	sub, err := client.SubscribeWithContext(ctx, &opcua.SubscriptionParameters{Interval: interval}, notifyCh)
	if err != nil {
		return err
	}
	defer sub.Cancel(ctx)

	klog.Infof("Created subscription with id %v", sub.SubscriptionID)

	for index, deviceResource := range config.Profile.DeviceResources {
		req, err := d.toRequest(config.Name, index, deviceResource)
		if err != nil {
			return err
		}

		resp, err := sub.Monitor(ua.TimestampsToReturnBoth, valueRequest(req))
		if err != nil || resp.Results[0].StatusCode != ua.StatusOK {
			return err
		}
	}

	// read from subscription's notification channel until ctx is cancelled
	for {
		select {
		case <-ctx.Done():
			return nil
		case res := <-notifyCh:
			if res.Error != nil {
				klog.Errorf("%v", res.Error)
				continue
			}

			switch x := res.Value.(type) {
			case *ua.DataChangeNotification:
				for _, item := range x.MonitoredItems {
					data := item.Value.Value.Value()
					klog.Infof("MonitoredItem with client handle %v = %v", item.ClientHandle, data)

					req := d.findRequest(config.Name, item.ClientHandle)
					if req == nil {
						continue
					}

					result, err := util.NewResult(req.res, data)
					if err != nil {
						klog.Errorf("The device %s attribute %s  is unsupported, %v", config.Name, req.res.Name, err)
						continue
					}

					for _, msgBus := range d.msgBuses {
						msgBus.ReceiveData(config.Name, *result)
					}
				}

			case *ua.EventNotificationList:
				// do nothing
			default:
				klog.Infof("unknown publish result: %T", res.Value)
			}
		}
	}
}

func (d *OPCUADriver) findEndpoint(config v1alpha1.DeviceConfig) (string, error) {
	protocolProperties := config.ProtocolProperties
	endpoint, ok := protocolProperties.Data[Endpoint]
	if !ok {
		return "", fmt.Errorf("endpoint not found in the opcua protocol properties, %v", protocolProperties.Data)
	}
	return fmt.Sprintf("%v", endpoint), nil
}

func (d *OPCUADriver) toRequest(deviceName string, index int, res v1alpha1.DeviceResource) (*request, error) {
	nodeId, err := getNodeID(res.Attributes, NODE)
	if err != nil {
		return nil, err
	}

	id, err := ua.ParseNodeID(nodeId)
	if err != nil {
		return nil, err
	}

	req := request{
		nodeId: id,
		handle: uint32(index + 42),
		res:    res,
	}

	device, ok := d.devices[deviceName]
	if !ok {
		return nil, fmt.Errorf("the device %s is not added", deviceName)
	}

	device.requests = append(device.requests, req)
	d.devices[deviceName] = device
	return &req, nil
}

func (d *OPCUADriver) findRequest(deviceName string, handle uint32) *request {
	device, ok := d.devices[deviceName]
	if !ok {
		return nil
	}

	for _, req := range device.requests {
		if req.handle == handle {
			return &req
		}
	}

	return nil
}

func valueRequest(req *request) *ua.MonitoredItemCreateRequest {
	return opcua.NewMonitoredItemCreateRequestWithDefaults(req.nodeId, ua.AttributeIDValue, req.handle)
}

func getNodeID(attrs v1alpha1.Values, id string) (string, error) {
	identifier, ok := attrs.Data[id]
	if !ok {
		return "", fmt.Errorf("attribute %s does not exist", id)
	}

	return identifier.(string), nil
}
