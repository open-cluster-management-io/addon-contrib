package spoke

import (
	"context"
	"errors"

	"github.com/clusternet/clusternet/pkg/controllers/proxies/sockets"
	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"github.com/spf13/pflag"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"open-cluster-management.io/addon-framework/pkg/lease"

	"open-cluster-management.io/addon-contrib/clusternet-addon/pkg/helpers"
)

type AgentOptions struct {
	InstallationNamespace string
	HubKubeconfigFile     string
	ClusterName           string
}

func NewAgentOptions() *AgentOptions {
	return &AgentOptions{}
}

func (o *AgentOptions) AddFlags(flags *pflag.FlagSet) {
	flags.StringVar(&o.HubKubeconfigFile, "hub-kubeconfig", o.HubKubeconfigFile, "Location of kubeconfig file to connect to hub cluster.")
	flags.StringVar(&o.ClusterName, "cluster-name", o.ClusterName, "Name of managed cluster.")
}

func (o *AgentOptions) Complete() {
	o.InstallationNamespace = helpers.GetCurrentNamespace(helpers.DefaultInstallationNamespace)
}

func (o *AgentOptions) Validate() error {
	if o.HubKubeconfigFile == "" {
		return errors.New("hub-kubeconfig is required")
	}

	if o.ClusterName == "" {
		return errors.New("cluster name is empty")
	}

	return nil
}

func (o *AgentOptions) RunAgent(ctx context.Context, controllerContext *controllercmd.ControllerContext) error {
	o.Complete()

	if err := o.Validate(); err != nil {
		return err
	}

	hubRestConfig, err := clientcmd.BuildConfigFromFlags("", o.HubKubeconfigFile)
	if err != nil {
		return err
	}

	spokeKubeClient, err := kubernetes.NewForConfig(controllerContext.KubeConfig)
	if err != nil {
		return err
	}

	// start websocket connection
	socketConn, err := sockets.NewController(hubRestConfig, true)
	if err != nil {
		return err

	}
	clusterID := types.UID(o.ClusterName)
	go socketConn.Run(ctx, &clusterID)

	// start lease updater
	leaseUpdater := lease.NewLeaseUpdater(
		spokeKubeClient,
		helpers.AddOnName,
		o.InstallationNamespace,
	)
	go leaseUpdater.Start(ctx)

	<-ctx.Done()
	return nil
}
