package agent

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	corev1informers "k8s.io/client-go/informers/core/v1"
	clusterclient "open-cluster-management.io/api/client/cluster/clientset/versioned"
	"open-cluster-management.io/api/client/cluster/listers/cluster/v1alpha1"

	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"open-cluster-management.io/addon-framework/pkg/lease"
	"open-cluster-management.io/addon-framework/pkg/version"
	addonv1alpha1client "open-cluster-management.io/api/client/addon/clientset/versioned"
	clientset "open-cluster-management.io/api/client/cluster/clientset/versioned"
	clusterinformers "open-cluster-management.io/api/client/cluster/informers/externalversions"
	clusterinformers1 "open-cluster-management.io/api/client/cluster/informers/externalversions/cluster/v1alpha1"
	apiv1alpha2 "open-cluster-management.io/api/cluster/v1alpha1"
)

const AgentInstallationNamespace = "default"
const AddOnPlacementScoresName = "resource-usage-score"

func NewAgentCommand(addonName string) *cobra.Command {
	o := NewAgentOptions(addonName)
	cmd := controllercmd.
		NewControllerCommandConfig("resource-usage-collection-addon-agent", version.Get(), o.RunAgent).
		NewCommand()
	cmd.Use = "agent"
	cmd.Short = "Start the addon agent"

	o.AddFlags(cmd)
	return cmd
}

// AgentOptions defines the flags for workload agent
type AgentOptions struct {
	HubKubeconfigFile string
	SpokeClusterName  string
	AddonName         string
	AddonNamespace    string
}

// NewWorkloadAgentOptions returns the flags with default value set
func NewAgentOptions(addonName string) *AgentOptions {
	return &AgentOptions{AddonName: addonName}
}

func (o *AgentOptions) AddFlags(cmd *cobra.Command) {
	flags := cmd.Flags()
	// This command only supports reading from config
	flags.StringVar(&o.HubKubeconfigFile, "hub-kubeconfig", o.HubKubeconfigFile, "Location of kubeconfig file to connect to hub cluster.")
	flags.StringVar(&o.SpokeClusterName, "cluster-name", o.SpokeClusterName, "Name of spoke cluster.")
	flags.StringVar(&o.AddonNamespace, "addon-namespace", o.AddonNamespace, "Installation namespace of addon.")
}

// RunAgent starts the controllers on agent to process work from hub.
func (o *AgentOptions) RunAgent(ctx context.Context, controllerContext *controllercmd.ControllerContext) error {
	// build kubeclient of managed cluster
	spokeKubeClient, err := kubernetes.NewForConfig(controllerContext.KubeConfig)
	if err != nil {
		return err
	}

	// build kubeinformerfactory of hub cluster
	hubRestConfig, err := clientcmd.BuildConfigFromFlags("" /* leave masterurl as empty */, o.HubKubeconfigFile)
	if err != nil {
		return err
	}
	// ++2
	hubClusterClient, err := clusterclient.NewForConfig(hubRestConfig)
	if err != nil {
		return nil
	}

	if err != nil {
		return err
	}
	spokeKubeInformerFactory := informers.NewSharedInformerFactory(spokeKubeClient, 10*time.Minute)
	// ++4
	clusterInformers := clusterinformers.NewSharedInformerFactoryWithOptions(hubClusterClient, 10*time.Minute, clusterinformers.WithNamespace(o.SpokeClusterName))

	// create an agent controller
	agent := newAgentController(
		spokeKubeClient,
		hubClusterClient,
		clusterInformers.Cluster().V1alpha1().AddOnPlacementScores(),
		o.SpokeClusterName,
		o.AddonName,
		o.AddonNamespace,
		controllerContext.EventRecorder,
		spokeKubeInformerFactory.Core().V1().Nodes(),
		spokeKubeInformerFactory.Core().V1().Pods(),
	)
	// create a lease updater
	leaseUpdater := lease.NewLeaseUpdater(
		spokeKubeClient,
		o.AddonName,
		o.AddonNamespace,
	)

	// go hubKubeInformerFactory.Start(ctx.Done())
	go clusterInformers.Start(ctx.Done())
	go spokeKubeInformerFactory.Start(ctx.Done())
	go agent.Run(ctx, 1)
	go leaseUpdater.Start(ctx)

	<-ctx.Done()
	return nil
}

type agentController struct {
	spokeKubeClient           kubernetes.Interface
	hubKubeClient             clientset.Interface
	addonClient               addonv1alpha1client.Interface
	AddOnPlacementScoreLister v1alpha1.AddOnPlacementScoreLister
	clusterName               string
	addonName                 string
	addonNamespace            string
	recorder                  events.Recorder
	nodeInformer              corev1informers.NodeInformer
	podInformer               corev1informers.PodInformer
}

func newAgentController(
	spokeKubeClient kubernetes.Interface,
	hubKubeClient clientset.Interface,
	addOnPlacementScoreInformer clusterinformers1.AddOnPlacementScoreInformer,
	clusterName string,
	addonName string,
	addonNamespace string,
	recorder events.Recorder,
	nodeInformer corev1informers.NodeInformer,
	podInformer corev1informers.PodInformer,
) factory.Controller {
	c := &agentController{
		spokeKubeClient:           spokeKubeClient,
		hubKubeClient:             hubKubeClient,
		clusterName:               clusterName,
		addonName:                 addonName,
		addonNamespace:            addonNamespace,
		AddOnPlacementScoreLister: addOnPlacementScoreInformer.Lister(),
		recorder:                  recorder,
		podInformer:               podInformer,
		nodeInformer:              nodeInformer,
	}
	return factory.New().WithInformersQueueKeyFunc(
		func(obj runtime.Object) string {
			key, _ := cache.MetaNamespaceKeyFunc(obj)
			return key
		}, addOnPlacementScoreInformer.Informer()).
		WithBareInformers(podInformer.Informer(), nodeInformer.Informer()).
		WithSync(c.sync).ResyncEvery(time.Second*60).ToController("score-agent-controller", recorder)
}

func (c *agentController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	score := NewScore(c.nodeInformer, c.podInformer)
	cpuScore, memScore, err := score.calculateScore()
	items := []apiv1alpha2.AddOnPlacementScoreItem{
		{
			Name:  "cpuAvailable",
			Value: int32(cpuScore),
		},
		{
			Name:  "memAvailable",
			Value: int32(memScore),
		},
	}

	addonPlacementScore, err := c.AddOnPlacementScoreLister.AddOnPlacementScores(c.clusterName).Get(AddOnPlacementScoresName)
	switch {
	case errors.IsNotFound(err):
		addonPlacementScore = &apiv1alpha2.AddOnPlacementScore{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: c.clusterName,
				Name:      AddOnPlacementScoresName,
			},
			Status: apiv1alpha2.AddOnPlacementScoreStatus{
				Scores: items,
			},
		}
		_, err = c.hubKubeClient.ClusterV1alpha1().AddOnPlacementScores(c.clusterName).Create(ctx, addonPlacementScore, v1.CreateOptions{})
		if err != nil {
			return err
		}
		return nil
	case err != nil:
		return err
	}

	addonPlacementScore.Status.Scores = items
	_, err = c.hubKubeClient.ClusterV1alpha1().AddOnPlacementScores(c.clusterName).UpdateStatus(ctx, addonPlacementScore, v1.UpdateOptions{})
	return err
}
