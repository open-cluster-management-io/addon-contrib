/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"fmt"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	"github.com/spf13/cobra"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

	"open-cluster-management.io/addon-contrib/example-addon/addonmanager"
	openclustermanagementiov1alpha1 "open-cluster-management.io/addon-contrib/example-addon/api/v1alpha1"
	"open-cluster-management.io/addon-contrib/example-addon/controllers"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(openclustermanagementiov1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	command := newCommand()
	if err := command.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func newCommand() *cobra.Command {
	controllerFlags := &addonmanager.ControllerFlags{}
	agentFlags := &AgentFlags{}
	cmd := &cobra.Command{
		Use:   "addon",
		Short: "helloworld example addon",
		Run: func(cmd *cobra.Command, args []string) {
			if err := cmd.Help(); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
			}
			os.Exit(1)
		},
	}

	cmd.AddCommand(controllerFlags.NewControllerCommand())
	cmd.AddCommand(agentFlags.NewCommand())

	return cmd
}

// AgentFlags provides the "normal" controller flags
type AgentFlags struct {
	// KubeConfigFile points to a kubeconfig file if you don't want to use the in cluster config
	KubeConfigFile string
	// HubKubeConfigFile to a kubeconfig file connect to hub
	HubKubeConfigFile string
	// ClusterName is the name of the cluster agent is running
	ClusterName string
}

func (f *AgentFlags) NewCommand() *cobra.Command {
	ctx := context.TODO()
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "helloworld example addon",
		Run: func(cmd *cobra.Command, args []string) {
			if err := f.startAgent(ctx); err != nil {
				klog.Fatal(err)
			}
		},
	}

	f.AddFlags(cmd)

	return cmd
}

// AddFlags register and binds the default flags
func (f *AgentFlags) AddFlags(cmd *cobra.Command) {
	flags := cmd.Flags()
	// This command only supports reading from config

	flags.StringVar(&f.KubeConfigFile, "kubeconfig", f.KubeConfigFile, "Location of the master configuration file to run from.")
	flags.StringVar(&f.HubKubeConfigFile, "hub-kubeconfig", f.HubKubeConfigFile, "Location of the hub configuration file to run from.")
	flags.StringVar(&f.ClusterName, "cluster", f.ClusterName, "Name of the cluster the agent is running.")
}

func (a *AgentFlags) startAgent(ctx context.Context) error {
	hubconfig, err := clientcmd.BuildConfigFromFlags("", a.HubKubeConfigFile)
	if err != nil {
		return err
	}

	mgr, err := ctrl.NewManager(hubconfig, ctrl.Options{
		Scheme:         scheme,
		LeaderElection: false,
		Namespace:      a.ClusterName,
	})
	if err != nil {
		return err
	}

	if err = (&controllers.HelloSpokeReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}
	//+kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return err
	}

	return nil
}
