package hub

import (
	"context"

	utilrand "k8s.io/apimachinery/pkg/util/rand"

	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"open-cluster-management.io/addon-contrib/clusternet-addon/pkg/helpers"
	"open-cluster-management.io/addon-contrib/clusternet-addon/pkg/hub/addon"

	"k8s.io/klog/v2"

	"open-cluster-management.io/addon-framework/pkg/addonfactory"
	"open-cluster-management.io/addon-framework/pkg/addonmanager"
	"open-cluster-management.io/addon-framework/pkg/agent"
)

type AddOnControllerOptions struct{}

func NewAddOnControllerOptions() *AddOnControllerOptions {
	return &AddOnControllerOptions{}
}

// RunControllerManager starts the clusternet add-on controller on hub.
func (o *AddOnControllerOptions) RunControllerManager(ctx context.Context, controllerContext *controllercmd.ControllerContext) error {
	kubeConfig := controllerContext.KubeConfig

	mgr, err := addonmanager.New(kubeConfig)
	if err != nil {
		return err
	}

	registrationOption := addon.NewRegistrationOption(
		ctx,
		kubeConfig,
		helpers.AddOnName,
		utilrand.String(5),
	)

	agentAddon, err := addonfactory.NewAgentAddonFactory(helpers.AddOnName, addon.FS, "manifests").
		WithGetValuesFuncs(addon.GetDefaultValues).
		WithAgentRegistrationOption(registrationOption).
		WithInstallStrategy(agent.InstallAllStrategy(helpers.DefaultInstallationNamespace)).
		BuildTemplateAgentAddon()
	if err != nil {
		klog.Fatalf("failed to build agent %v", err)
	}

	if err = mgr.AddAgent(agentAddon); err != nil {
		klog.Fatalf("failed to add agent", err)
	}

	if err := mgr.Start(ctx); err != nil {
		klog.Fatalf("failed to start addon manager", err)
	}

	<-ctx.Done()
	return nil
}
