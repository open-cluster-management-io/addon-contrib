package manager

import (
	"context"
	"embed"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"github.com/openshift/library-go/pkg/assets"
	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/component-base/version"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ocmv1alpha1 "open-cluster-management.io/addon-contrib/hellospoke-addon/api/v1alpha1"
	"open-cluster-management.io/addon-contrib/hellospoke-addon/pkg/constants"
	"open-cluster-management.io/addon-framework/pkg/addonfactory"
	"open-cluster-management.io/addon-framework/pkg/addonmanager"
	frameworkagent "open-cluster-management.io/addon-framework/pkg/agent"
	"open-cluster-management.io/addon-framework/pkg/utils"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

var (
	genericScheme = runtime.NewScheme()
	genericCodecs = serializer.NewCodecFactory(genericScheme)
	genericCodec  = genericCodecs.UniversalDeserializer()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(genericScheme))
	utilruntime.Must(ocmv1alpha1.AddToScheme(genericScheme))
}

const (
	templatePath = "manifests/templates"
)

//go:embed manifests
//go:embed manifests/templates
var fs embed.FS

var agentPermissionFiles = []string{
	// role with RBAC rules to access resources on hub
	"manifests/permission/role.yaml",
	// rolebinding to bind the above role to a certain user group
	"manifests/permission/rolebinding.yaml",
}

type override struct {
	client.Client
	log               logr.Logger
	operatorNamespace string
	withOverride      bool
}

func NewManagerCommand(componentName string, log logr.Logger) *cobra.Command {
	var withOverride bool
	runController := func(ctx context.Context, controllerContext *controllercmd.ControllerContext) error {
		mgr, err := addonmanager.New(controllerContext.KubeConfig)
		if err != nil {
			return err
		}
		registrationOption := newRegistrationOption(
			controllerContext.KubeConfig,
			controllerContext.EventRecorder,
			componentName,
			utilrand.String(5),
		)

		hubClient, err := client.New(controllerContext.KubeConfig, client.Options{Scheme: genericScheme})
		if err != nil {
			log.Error(err, "failed to create hub client to fetch downstream image override configmap")
			return err
		}

		o := &override{
			Client:            hubClient,
			log:               log.WithName("override-values"),
			operatorNamespace: controllerContext.OperatorNamespace,
			withOverride:      withOverride,
		}

		agentAddon, err := addonfactory.NewAgentAddonFactory(componentName, fs, templatePath).
			WithGetValuesFuncs(o.getValueForAgentTemplate, addonfactory.GetValuesFromAddonAnnotation).
			WithAgentRegistrationOption(registrationOption).
			WithInstallStrategy(frameworkagent.InstallAllStrategy(constants.AgentInstallationNamespace)).
			BuildTemplateAgentAddon()
		if err != nil {
			log.Error(err, "failed to build agent")
			return err
		}

		err = mgr.AddAgent(agentAddon)
		if err != nil {
			log.Error(err, "failed to add agent")
			os.Exit(1)
		}

		err = mgr.Start(ctx)
		if err != nil {
			log.Error(err, "failed to start addon framework manager")
			os.Exit(1)
		}
		<-ctx.Done()

		return nil
	}

	cmdConfig := controllercmd.
		NewControllerCommandConfig(componentName, version.Get(), runController)

	cmd := cmdConfig.NewCommand()
	cmd.Use = "manager"
	cmd.Short = fmt.Sprintf("Start the %s's manager", componentName)

	// add disable leader election flag
	flags := cmd.Flags()
	flags.BoolVar(&cmdConfig.DisableLeaderElection, "disable-leader-election", true, "Disable leader election for the agent.")
	flags.BoolVar(&withOverride, "with-image-override", false, "Use image from override configmap")

	return cmd
}

func newRegistrationOption(kubeConfig *rest.Config, recorder events.Recorder, componentName, agentName string) *frameworkagent.RegistrationOption {
	return &frameworkagent.RegistrationOption{
		CSRConfigurations: frameworkagent.KubeClientSignerConfigurations(componentName, agentName),
		CSRApproveCheck:   utils.DefaultCSRApprover(agentName),
		PermissionConfig: func(cluster *clusterv1.ManagedCluster, addon *addonapiv1alpha1.ManagedClusterAddOn) error {
			kubeclient, err := kubernetes.NewForConfig(kubeConfig)
			if err != nil {
				return err
			}

			for _, file := range agentPermissionFiles {
				if err := applyAgentPermissionManifestFromFile(file, cluster.Name, addon.Name, kubeclient, recorder); err != nil {
					return err
				}
			}

			return nil
		},
	}
}

func applyAgentPermissionManifestFromFile(file, clusterName, componentName string, kubeclient *kubernetes.Clientset, recorder events.Recorder) error {
	groups := frameworkagent.DefaultGroups(clusterName, componentName)
	config := struct {
		ClusterName            string
		Group                  string
		RoleAndRolebindingName string
	}{
		ClusterName: clusterName,

		Group:                  groups[0],
		RoleAndRolebindingName: fmt.Sprintf("open-cluster-management:%s:agent", componentName),
	}

	results := resourceapply.ApplyDirectly(
		context.Background(),
		resourceapply.NewKubeClientHolder(kubeclient),
		recorder,
		resourceapply.NewResourceCache(),
		func(name string) ([]byte, error) {
			template, err := fs.ReadFile(file)
			if err != nil {
				return nil, err
			}

			data := assets.MustCreateAssetFromTemplate(name, template, config).Data

			return data, nil
		},
		file,
	)

	for _, result := range results {
		if result.Error != nil {
			return result.Error
		}
	}

	return nil
}

// getValues prepare values for templates at manifests/templates
func (o *override) getValueForAgentTemplate(cluster *clusterv1.ManagedCluster,
	addon *addonapiv1alpha1.ManagedClusterAddOn) (addonfactory.Values, error) {
	addonImage := os.Getenv("ADDON_IMAGE")
	if len(addonImage) == 0 {
		addonImage = constants.AddonImage
	}

	manifestConfig := struct {
		KubeConfigSecret        string
		ClusterName             string
		AddonName               string
		AddonInstallNamespace   string
		Image                   string
		SpokeRolebindingName    string
		AgentServiceAccountName string
	}{
		KubeConfigSecret:        fmt.Sprintf("%s-hub-kubeconfig", addon.Name),
		AddonInstallNamespace:   constants.AgentInstallationNamespace,
		ClusterName:             cluster.Name,
		AddonName:               fmt.Sprintf("%s-agent", addon.Name),
		Image:                   addonImage,
		SpokeRolebindingName:    addon.Name,
		AgentServiceAccountName: fmt.Sprintf("%s-agent-sa", addon.Name),
	}

	return addonfactory.StructToValues(manifestConfig), nil
}
