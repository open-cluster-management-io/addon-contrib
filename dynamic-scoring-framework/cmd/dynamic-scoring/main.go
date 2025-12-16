package main

import (
	"context"
	goflag "flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/rest"
	utilflag "k8s.io/component-base/cli/flag"
	logs "k8s.io/component-base/logs/api/v1"
	"k8s.io/klog/v2"
	addonv1alpha1client "open-cluster-management.io/api/client/addon/clientset/versioned"

	"open-cluster-management.io/addon-framework/pkg/addonfactory"
	"open-cluster-management.io/addon-framework/pkg/addonmanager"
	"open-cluster-management.io/addon-framework/pkg/addonmanager/cloudevents"
	cmdfactory "open-cluster-management.io/addon-framework/pkg/cmd/factory"
	"open-cluster-management.io/addon-framework/pkg/utils"
	"open-cluster-management.io/addon-framework/pkg/version"

	"open-cluster-management.io/dynamic-scoring/pkg/dynamic_scoring"
	"open-cluster-management.io/dynamic-scoring/pkg/dynamic_scoring_agent"
)

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	pflag.CommandLine.SetNormalizeFunc(utilflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(goflag.CommandLine)

	logs.AddFlags(logs.NewLoggingConfiguration(), pflag.CommandLine)

	command := newCommand()
	if err := command.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func newCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dynamic-scoring-addon",
		Short: "dynamic scoring addon",
		Run: func(cmd *cobra.Command, args []string) {
			if err := cmd.Help(); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
			}
			os.Exit(1)
		},
	}

	if v := version.Get().String(); len(v) == 0 {
		cmd.Version = "<unknown>"
	} else {
		cmd.Version = v
	}

	cmd.AddCommand(newControllerCommand())
	cmd.AddCommand(dynamic_scoring_agent.NewAgentCommand(dynamic_scoring.AddonName))

	return cmd
}

func newControllerCommand() *cobra.Command {
	o := cloudevents.NewCloudEventsOptions()
	c := &addManagerConfig{cloudeventsOptions: o}
	cmd := cmdfactory.
		NewControllerCommandConfig("dynamic-scoring-addon-controller", version.Get(), c.runController).
		NewCommand()
	cmd.Use = "controller"
	cmd.Short = "Start the addon controller"
	o.AddFlags(cmd)

	return cmd
}

// addManagerConfig holds cloudevents configuration for addon manager
type addManagerConfig struct {
	cloudeventsOptions *cloudevents.CloudEventsOptions
}

func (c *addManagerConfig) runController(ctx context.Context, kubeConfig *rest.Config) error {
	addonClient, err := addonv1alpha1client.NewForConfig(kubeConfig)
	if err != nil {
		return err
	}

	var mgr addonmanager.AddonManager
	if c.cloudeventsOptions.WorkDriver == "kube" {
		mgr, err = addonmanager.New(kubeConfig)
		if err != nil {
			return err
		}
	} else {
		mgr, err = cloudevents.New(kubeConfig, c.cloudeventsOptions)
		if err != nil {
			return err
		}
	}

	registrationOption := dynamic_scoring.NewRegistrationOption(
		kubeConfig,
		dynamic_scoring.AddonName,
		utilrand.String(5),
	)

	// Set agent install namespace from addon deployment config if it exists
	registrationOption.AgentInstallNamespace = utils.AgentInstallNamespaceFromDeploymentConfigFunc(
		utils.NewAddOnDeploymentConfigGetter(addonClient),
	)

	agentAddon, err := addonfactory.NewAgentAddonFactory(dynamic_scoring.AddonName, dynamic_scoring.FS, "manifests/templates").
		WithConfigGVRs(utils.AddOnDeploymentConfigGVR).
		WithGetValuesFuncs(
			dynamic_scoring.GetDefaultValues,
			addonfactory.GetAddOnDeploymentConfigValues(
				utils.NewAddOnDeploymentConfigGetter(addonClient),
				addonfactory.ToAddOnDeploymentConfigValues,
				addonfactory.ToImageOverrideValuesFunc("Image", dynamic_scoring.DefaultDynamicScoringAddonImage),
			),
		).
		WithAgentRegistrationOption(registrationOption).
		WithAgentInstallNamespace(
			utils.AgentInstallNamespaceFromDeploymentConfigFunc(
				utils.NewAddOnDeploymentConfigGetter(addonClient),
			),
		).
		WithAgentHealthProber(dynamic_scoring.AgentHealthProber()).
		BuildTemplateAgentAddon()
	if err != nil {
		klog.Errorf("failed to build agent %v", err)
		return err
	}

	err = mgr.AddAgent(agentAddon)
	if err != nil {
		klog.Fatal(err)
	}

	err = mgr.Start(ctx)
	if err != nil {
		klog.Fatal(err)
	}
	<-ctx.Done()

	return nil
}
