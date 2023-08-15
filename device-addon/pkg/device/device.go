package device

import (
	"context"
	"path"

	"github.com/spf13/pflag"

	"open-cluster-management-io/addon-contrib/device-addon/pkg/apis/v1alpha1"
	"open-cluster-management-io/addon-contrib/device-addon/pkg/device/equipment"
	"open-cluster-management-io/addon-contrib/device-addon/pkg/device/util"
)

const (
	configFileName        = "config.yaml"
	driversConfigFileName = "drivers.yaml"
	devicesConfigFileName = "devices.yaml"
)

type DriverAgentOptions struct {
	ConfigDir string
}

type messageBusList struct {
	MessageBuses []v1alpha1.MessageBusConfig `yaml:"messageBuses"`
}

type driverList struct {
	Drivers []v1alpha1.DriverConfig `yaml:"drivers"`
}

type deviceList struct {
	Devices []v1alpha1.DeviceConfig `yaml:"devices"`
}

func NewDriverAgentOptions() *DriverAgentOptions {
	return &DriverAgentOptions{}
}

func (o *DriverAgentOptions) AddFlags(flags *pflag.FlagSet) {
	flags.StringVar(&o.ConfigDir, "config-dir", o.ConfigDir, "Directory of config files")
}

// RunAgent starts the controllers on agent to process work from hub.
// TODO watch the configuration files changes with "github.com/fsnotify/fsnotify"
func (o *DriverAgentOptions) RunDriverAgent(ctx context.Context) error {
	config := &messageBusList{}
	if err := util.LoadConfig(path.Join(o.ConfigDir, configFileName), config); err != nil {
		return err
	}

	driverList := &driverList{}
	if err := util.LoadConfig(path.Join(o.ConfigDir, driversConfigFileName), driverList); err != nil {
		return err
	}

	deviceList := &deviceList{}
	if err := util.LoadConfig(path.Join(o.ConfigDir, devicesConfigFileName), deviceList); err != nil {
		return err
	}

	e := equipment.NewEquipment()

	if err := e.Start(ctx, config.MessageBuses); err != nil {
		return err
	}

	for _, driver := range driverList.Drivers {
		if err := e.InstallDriver(driver); err != nil {
			return err
		}
	}

	for _, device := range deviceList.Devices {
		d := e.GetDriver(device.DriverType)
		if d != nil {
			d.AddDevice(device)
		}
	}

	<-ctx.Done()
	return nil
}
