package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	"open-cluster-management.io/addon-framework/pkg/lease"
	"otel-add-on/pkg/common"

	ctrl "sigs.k8s.io/controller-runtime"
)

const envKeyPodNamespace = "POD_NAMESPACE"

var (
	hubKubeconfig string
	clusterName   string
)

func main() {
	logger := klogr.New()
	klog.SetOutput(os.Stdout)
	klog.InitFlags(flag.CommandLine)
	flag.StringVar(&hubKubeconfig, "hub-kubeconfig", "", "The kubeconfig to talk to hub cluster")
	flag.StringVar(&clusterName, "cluster-name", "", "The name of the managed cluster")
	flag.Parse()

	// pipe controller-runtime logs to klog
	ctrl.SetLogger(logger)

	cfg, err := clientcmd.BuildConfigFromFlags("", hubKubeconfig)
	if err != nil {
		panic(err)
	}

	spokeClient, err := kubernetes.NewForConfig(ctrl.GetConfigOrDie())

	cfg.UserAgent = "otel-collector-addon-agent"

	if err != nil {
		panic(fmt.Errorf("failed to create spoke client, err: %w", err))
	}
	addonAgentNamespace := os.Getenv("POD_NAMESPACE")
	if len(addonAgentNamespace) == 0 {
		panic(fmt.Sprintf("Pod namespace is empty, please set the ENV for %s", envKeyPodNamespace))
	}

	leaseUpdater := lease.NewLeaseUpdater(spokeClient, common.AddonName, addonAgentNamespace).
		WithHubLeaseConfig(cfg, clusterName)

	ctx := context.Background()
	klog.Infof("Starting lease updater")
	leaseUpdater.Start(ctx)
	<-ctx.Done()

}
