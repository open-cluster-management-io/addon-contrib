package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	goflag "flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes"
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
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"open-cluster-management.io/dynamic-scoring/pkg/dynamic_scoring"
	"open-cluster-management.io/dynamic-scoring/pkg/dynamic_scoring_agent"
)

const (
	// Hub side (source) pull secret location.
	hubPullSecretNamespace = "open-cluster-management"
	hubPullSecretName      = "dynamic-scoring-addon-pull-secret"

	// Managed side (target) pull secret name (created in the addon install namespace).
	managedPullSecretName = "hub-registry-secret"
)

// normalizeAddOnDeploymentConfigValuesFunc converts certain AddOnDeploymentConfig customizedVariables
// from string form into the types expected by the go-template manifests.
//
// Today we support:
// - ImagePullSecrets: JSON array string (e.g. '["a","b"]') -> []string
func normalizeAddOnDeploymentConfigValuesFunc() addonfactory.AddOnDeploymentConfigToValuesFunc {
	return func(config addonapiv1alpha1.AddOnDeploymentConfig) (addonfactory.Values, error) {
		values, err := addonfactory.ToAddOnDeploymentConfigValues(config)
		if err != nil {
			return nil, err
		}

		// Convert ImagePullSecrets from string to []string so templates can "range" over it.
		if raw, ok := values["ImagePullSecrets"]; ok {
			if s, ok := raw.(string); ok {
				if len(s) == 0 {
					delete(values, "ImagePullSecrets")
					return values, nil
				}
				var secrets []string
				if err := json.Unmarshal([]byte(s), &secrets); err == nil {
					values["ImagePullSecrets"] = secrets
				}
			}
		}

		return values, nil
	}
}

// hubPullSecretValuesFunc reads a dockerconfigjson Secret from the hub cluster and passes it as template values
// so a pull secret can be created on the managed cluster via ManifestWork.
//
// This is intentionally best-effort: if the secret doesn't exist (or is malformed), we don't fail the addon.
func hubPullSecretValuesFunc(kubeClient kubernetes.Interface) addonfactory.GetValuesFunc {
	return func(cluster *clusterv1.ManagedCluster, addon *addonapiv1alpha1.ManagedClusterAddOn) (addonfactory.Values, error) {
		secret, err := kubeClient.CoreV1().Secrets(hubPullSecretNamespace).Get(context.TODO(), hubPullSecretName, metav1.GetOptions{})
		if err != nil {
			klog.V(2).Infof("hub pull secret %s/%s not available: %v", hubPullSecretNamespace, hubPullSecretName, err)
			klog.Info("hub pull secret ", hubPullSecretNamespace, "/", hubPullSecretName, " not available")
			return nil, nil
		}
		if secret.Type != corev1.SecretTypeDockerConfigJson {
			klog.Info("hub pull secret ", hubPullSecretNamespace, "/", hubPullSecretName, " has unexpected type ", secret.Type, " (expected ", corev1.SecretTypeDockerConfigJson, ")")
			klog.Warningf("hub pull secret %s/%s has unexpected type %q (expected %q)", hubPullSecretNamespace, hubPullSecretName, secret.Type, corev1.SecretTypeDockerConfigJson)
			return nil, nil
		}
		b := secret.Data[corev1.DockerConfigJsonKey]
		if len(b) == 0 {
			klog.Info("hub pull secret ", hubPullSecretNamespace, "/", hubPullSecretName, " missing ", corev1.DockerConfigJsonKey, " key")
			klog.Warningf("hub pull secret %s/%s missing %q key", hubPullSecretNamespace, hubPullSecretName, corev1.DockerConfigJsonKey)
			return nil, nil
		}

		klog.Info("hub pull secret ", hubPullSecretNamespace, "/", hubPullSecretName, " available")

		return addonfactory.Values{
			"PullSecretName":             managedPullSecretName,
			"PullSecretDockerConfigJson": base64.StdEncoding.EncodeToString(b),
		}, nil
	}
}

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
	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return err
	}

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

	klog.Info("agent install namespace for AddOn ", dynamic_scoring.AddonName)

	agentAddon, err := addonfactory.NewAgentAddonFactory(dynamic_scoring.AddonName, dynamic_scoring.FS, "manifests/templates").
		WithConfigGVRs(utils.AddOnDeploymentConfigGVR).
		WithGetValuesFuncs(
			dynamic_scoring.GetDefaultValues,
			hubPullSecretValuesFunc(kubeClient),
			addonfactory.GetAddOnDeploymentConfigValues(
				utils.NewAddOnDeploymentConfigGetter(addonClient),
				normalizeAddOnDeploymentConfigValuesFunc(),
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
