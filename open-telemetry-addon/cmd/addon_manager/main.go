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
	"flag"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	"k8s.io/client-go/informers"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/klog"
	"k8s.io/klog/v2/klogr"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"open-cluster-management.io/addon-framework/pkg/addonmanager"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"open-cluster-management.io/api/client/addon/clientset/versioned"
	"open-cluster-management.io/api/client/addon/informers/externalversions"

	"otel-add-on/pkg/agent"
	"otel-add-on/pkg/hub"

	"otel-add-on/pkg/config"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(addonv1alpha1.AddToScheme(scheme))

	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	logger := klogr.New()
	klog.SetOutput(os.Stdout)
	klog.InitFlags(flag.CommandLine)
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&config.AgentImageName, "agent-image-name",
		config.AgentImageName,
		"The name of the addon agent's image")
	flag.Parse()

	ctrl.SetLogger(logger)
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "otel-collector-addon-manager",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	client, err := versioned.NewForConfig(mgr.GetConfig())
	if err != nil {
		setupLog.Error(err, "unable to set up addon client")
		os.Exit(1)
	}

	nativeClient, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		setupLog.Error(err, "unable to set up kubernetes native client")
		os.Exit(1)
	}

	informerFactory := externalversions.NewSharedInformerFactory(client, 0)
	nativeInformer := informers.NewSharedInformerFactoryWithOptions(nativeClient, 0)

	//+kubebuilder:scaffold:builder
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}
	addonManager, err := addonmanager.New(mgr.GetConfig())
	if err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	err = hub.Applymanifests(mgr.GetClient())
	if err != nil {
		setupLog.Error(err, "unable to deploy hub manifests")
		os.Exit(1)
	}

	otelcollectoraddon, err := agent.NewAgentAddon(
		mgr.GetClient(),
		nativeClient)
	if err != nil {
		setupLog.Error(err, "unable to instantiate otel-collector addon")
		os.Exit(1)
	}

	if err := addonManager.AddAgent(otelcollectoraddon); err != nil {
		setupLog.Error(err, "unable to register otel-collector addon")
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(ctrl.SetupSignalHandler())
	defer cancel()
	go informerFactory.Start(ctx.Done())
	go nativeInformer.Start(ctx.Done())
	go func() {
		if err := addonManager.Start(ctx); err != nil {
			setupLog.Error(err, "unable to start addon manager")
			os.Exit(1)
		}
	}()
	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
