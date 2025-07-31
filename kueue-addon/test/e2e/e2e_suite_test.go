package e2e

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clusterv1client "open-cluster-management.io/api/client/cluster/clientset/versioned"
	kueueclientset "sigs.k8s.io/kueue/client-go/clientset/versioned"
)

var (
	// kubeconfigs
	hubKubeconfig string

	// clients
	hubClient        kubernetes.Interface
	hubClusterClient clusterv1client.Interface
	hubKueueClient   kueueclientset.Interface

	// test context
	ctx context.Context
)

func init() {
	flag.StringVar(&hubKubeconfig, "hub-kubeconfig", "", "The kubeconfig of the hub cluster")
}

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kueue Addon E2E Suite")
}

// This suite is sensitive to the following environment variables:
//
// - KUBECONFIG is the location of the kubeconfig file to use
var _ = BeforeSuite(func() {
	var err error

	// Setup context
	ctx = context.Background()

	// Setup kubeconfig with environment variable fallback
	if hubKubeconfig == "" {
		hubKubeconfig = os.Getenv("KUBECONFIG")
	}

	// Load kubeconfig
	var config *rest.Config
	config, err = clientcmd.BuildConfigFromFlags("", hubKubeconfig)
	Expect(err).NotTo(HaveOccurred())

	// Create clients
	hubClient, err = kubernetes.NewForConfig(config)
	Expect(err).NotTo(HaveOccurred())

	hubClusterClient, err = clusterv1client.NewForConfig(config)
	Expect(err).NotTo(HaveOccurred())

	hubKueueClient, err = kueueclientset.NewForConfig(config)
	Expect(err).NotTo(HaveOccurred())

	By("Check Kueue System Ready")
	// Wait for kueue-system namespace to be ready
	Eventually(func() error {
		_, err := hubClient.CoreV1().Namespaces().Get(ctx, "kueue-system", metav1.GetOptions{})
		return err
	}).Should(Succeed())

	// Wait for kueue controller to be ready
	Eventually(func() error {
		pods, err := hubClient.CoreV1().Pods("kueue-system").List(ctx, metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=kueue",
		})
		if err != nil {
			return err
		}
		if len(pods.Items) == 0 {
			return fmt.Errorf("no kueue controller pods found")
		}
		for _, pod := range pods.Items {
			if pod.Status.Phase != corev1.PodRunning {
				return fmt.Errorf("kueue controller pod %s is not running: %s", pod.Name, pod.Status.Phase)
			}
		}
		return nil
	}).Should(Succeed())
})

var _ = AfterSuite(func() {
	// Cleanup any remaining test resources
	By("Cleaning up test resources")

	// Delete any remaining test resources
	cleanupTestResources()
})

func cleanupTestResources() {
	// Delete Jobs
	jobs, err := hubClient.BatchV1().Jobs("default").List(ctx, metav1.ListOptions{
		LabelSelector: "test-scenario",
	})
	if err == nil {
		for _, job := range jobs.Items {
			err := hubClient.BatchV1().Jobs("default").Delete(ctx, job.Name, metav1.DeleteOptions{})
			if err != nil {
				fmt.Printf("Error deleting job %s: %v\n", job.Name, err)
			}
		}
	}

	// Delete workloads
	for _, job := range jobs.Items {
		workloads, err := hubKueueClient.KueueV1beta1().Workloads("default").List(ctx, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("kueue.x-k8s.io/job-uid=%s", job.UID),
		})
		if err == nil {
			for _, workload := range workloads.Items {
				err := hubKueueClient.KueueV1beta1().Workloads("default").Delete(ctx, workload.Name, metav1.DeleteOptions{})
				if err != nil {
					fmt.Printf("Error deleting workload %s: %v\n", workload.Name, err)
				}
			}
		}
	}

	// Delete ResourceFlavors
	flavors, err := hubKueueClient.KueueV1beta1().ResourceFlavors().List(ctx, metav1.ListOptions{
		LabelSelector: "test-scenario",
	})
	if err == nil {
		for _, flavor := range flavors.Items {
			err := hubKueueClient.KueueV1beta1().ResourceFlavors().Delete(ctx, flavor.Name, metav1.DeleteOptions{})
			if err != nil {
				fmt.Printf("Error deleting resource flavor %s: %v\n", flavor.Name, err)
			}
		}
	}

	// Delete ClusterQueues
	queues, err := hubKueueClient.KueueV1beta1().ClusterQueues().List(ctx, metav1.ListOptions{
		LabelSelector: "test-scenario",
	})
	if err == nil {
		for _, queue := range queues.Items {
			err := hubKueueClient.KueueV1beta1().ClusterQueues().Delete(ctx, queue.Name, metav1.DeleteOptions{})
			if err != nil {
				fmt.Printf("Error deleting cluster queue %s: %v\n", queue.Name, err)
			}
		}
	}

	// Delete LocalQueues
	localQueues, err := hubKueueClient.KueueV1beta1().LocalQueues("default").List(ctx, metav1.ListOptions{
		LabelSelector: "test-scenario",
	})
	if err == nil {
		for _, queue := range localQueues.Items {
			err := hubKueueClient.KueueV1beta1().LocalQueues("default").Delete(ctx, queue.Name, metav1.DeleteOptions{})
			if err != nil {
				fmt.Printf("Error deleting local queue %s: %v\n", queue.Name, err)
			}
		}
	}

	// Delete AdmissionChecks
	checks, err := hubKueueClient.KueueV1beta1().AdmissionChecks().List(ctx, metav1.ListOptions{
		LabelSelector: "test-scenario",
	})
	if err == nil {
		for _, check := range checks.Items {
			err := hubKueueClient.KueueV1beta1().AdmissionChecks().Delete(ctx, check.Name, metav1.DeleteOptions{})
			if err != nil {
				fmt.Printf("Error deleting admission check %s: %v\n", check.Name, err)
			}
		}
	}

	// Delete Placements
	placements, err := hubClusterClient.ClusterV1beta1().Placements("kueue-system").List(ctx, metav1.ListOptions{
		LabelSelector: "test-scenario",
	})
	if err == nil {
		for _, placement := range placements.Items {
			err := hubClusterClient.ClusterV1beta1().Placements("kueue-system").Delete(ctx, placement.Name, metav1.DeleteOptions{})
			if err != nil {
				fmt.Printf("Error deleting placement %s: %v\n", placement.Name, err)
			}
		}
	}
}
