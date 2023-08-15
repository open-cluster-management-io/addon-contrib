package spoke

import (
	"context"
	"time"

	"github.com/spf13/pflag"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"open-cluster-management-io/addon-contrib/device-addon/pkg/addon/spoke/controllers"
	"open-cluster-management-io/addon-contrib/device-addon/pkg/apis/v1alpha1"
	deviceaddonclientset "open-cluster-management-io/addon-contrib/device-addon/pkg/client/clientset/versioned"
	deviceaddoninformers "open-cluster-management-io/addon-contrib/device-addon/pkg/client/informers/externalversions"
	"open-cluster-management-io/addon-contrib/device-addon/pkg/device/equipment"
	"open-cluster-management-io/addon-contrib/device-addon/pkg/device/util"
)

const (
	defaultDataTopic     = "devices/+/data/+"
	defaultPayloadFormat = "jsonMap"
)

// AgentOptions defines the flags for workload agent
type AgentOptions struct {
	SpokeClusterName  string
	HubKubeConfigFile string
	AddOnConfigFile   string
}

type messageBusList struct {
	MessageBuses []v1alpha1.MessageBusConfig `yaml:"messageBuses" json:"messageBuses"`
}

// NewAgentOptions returns the flags with default value set
func NewAgentOptions() *AgentOptions {
	return &AgentOptions{}
}

func (o *AgentOptions) AddFlags(flags *pflag.FlagSet) {
	flags.StringVar(&o.SpokeClusterName, "cluster-name", o.SpokeClusterName, "Name of spoke cluster.")
	flags.StringVar(&o.HubKubeConfigFile, "hub-kubeconfig", o.HubKubeConfigFile, "Location of kubeconfig file to connect to hub cluster.")
	flags.StringVar(&o.AddOnConfigFile, "addonconfig", o.AddOnConfigFile, "Location of add-on config file.")
}

// RunAgent starts the controllers on agent to process work from hub.
func (o *AgentOptions) RunAgent(ctx context.Context, kubeconfig *rest.Config) error {
	hubRestConfig, err := clientcmd.BuildConfigFromFlags("", o.HubKubeConfigFile)
	if err != nil {
		return err
	}

	deviceClient, err := deviceaddonclientset.NewForConfig(hubRestConfig)
	if err != nil {
		return err
	}

	config, err := o.LoadAddOnConfig()
	if err != nil {
		return err
	}

	equipment := equipment.NewEquipment()
	if err := equipment.Start(ctx, config); err != nil {
		return err
	}

	deviceinformerFactory := deviceaddoninformers.NewSharedInformerFactory(deviceClient, 10*time.Minute)

	driverController := controllers.NewDriversController(
		o.SpokeClusterName,
		deviceClient,
		deviceinformerFactory.Edge().V1alpha1().Drivers(),
		equipment,
	)

	deviceController := controllers.NewDevicesController(
		o.SpokeClusterName,
		deviceClient,
		deviceinformerFactory.Edge().V1alpha1().Devices(),
		equipment,
	)

	go deviceinformerFactory.Start(ctx.Done())

	go deviceController.Run(ctx, 1)
	go driverController.Run(ctx, 1)

	<-ctx.Done()

	return nil
}

func (o *AgentOptions) LoadAddOnConfig() ([]v1alpha1.MessageBusConfig, error) {
	if len(o.AddOnConfigFile) != 0 {
		config := &messageBusList{}
		if err := util.LoadConfig(o.AddOnConfigFile, config); err != nil {
			return nil, err
		}

		return config.MessageBuses, nil
	}

	return []v1alpha1.MessageBusConfig{
		{
			MessageBusType: "mqtt",
			Enabled:        true,
			Properties: v1alpha1.Values{
				Data: map[string]interface{}{
					"dataTopic":     defaultDataTopic,
					"payloadFormat": defaultPayloadFormat,
				},
			},
		},
	}, nil
}
