package integration

import (
	"context"
	"fmt"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"open-cluster-management.io/addon-contrib/kueue-addon/pkg/hub/controllers/common"
	clusterv1client "open-cluster-management.io/api/client/cluster/clientset/versioned"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
	kueueclientset "sigs.k8s.io/kueue/client-go/clientset/versioned"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"
)

// assertMultiKueueConfigCreated asserts that MultiKueueConfig is created with correct cluster names
func assertMultiKueueConfigCreated(ctx context.Context, client kueueclientset.Interface, configName string, expectedClusters []string) {
	ginkgo.By(fmt.Sprintf("Asserting MultiKueueConfig %s is created with clusters %v", configName, expectedClusters))
	gomega.Eventually(func() error {
		mkconfig, err := client.KueueV1beta1().MultiKueueConfigs().Get(ctx, configName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get MultiKueueConfig %s: %v", configName, err)
		}

		// Build expected cluster names
		expectedClusterNames := make([]string, len(expectedClusters))
		for i, cluster := range expectedClusters {
			expectedClusterNames[i] = configName + "-" + cluster
		}

		// Convert slices to sets for easy comparison
		expectedSet := sets.New(expectedClusterNames...)
		actualSet := sets.New(mkconfig.Spec.Clusters...)

		// Check if sets are equal
		if !expectedSet.Equal(actualSet) {
			return fmt.Errorf("expected clusters %v, got clusters %v", expectedSet.UnsortedList(), actualSet.UnsortedList())
		}

		return nil
	}, eventuallyTimeout, eventuallyInterval).Should(gomega.Succeed())
}

// assertMultiKueueConfigNotExists asserts that MultiKueueConfig does not exist
func assertMultiKueueConfigNotExists(ctx context.Context, client kueueclientset.Interface, configName string) {
	ginkgo.By(fmt.Sprintf("Asserting MultiKueueConfig %s does not exist", configName))
	gomega.Eventually(func() bool {
		_, err := client.KueueV1beta1().MultiKueueConfigs().Get(ctx, configName, metav1.GetOptions{})
		return errors.IsNotFound(err)
	}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())
}

// assertMultiKueueClustersCreated asserts that MultiKueueClusters are created with correct names
func assertMultiKueueClustersCreated(ctx context.Context, client kueueclientset.Interface, configName string, expectedClusters []string) {
	ginkgo.By(fmt.Sprintf("Asserting MultiKueueClusters %s are created with clusters %v", configName, expectedClusters))
	gomega.Eventually(func() error {
		mkclusters, err := client.KueueV1beta1().MultiKueueClusters().List(ctx, metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("failed to list MultiKueueClusters: %v", err)
		}

		// Build expected cluster names
		expectedClusterNames := make([]string, len(expectedClusters))
		for i, cluster := range expectedClusters {
			expectedClusterNames[i] = configName + "-" + cluster
		}

		// Get actual cluster names
		actualClusterNames := make([]string, len(mkclusters.Items))
		for i, mkcluster := range mkclusters.Items {
			actualClusterNames[i] = mkcluster.Name
		}

		// Convert to sets for comparison
		expectedSet := sets.New(expectedClusterNames...)
		actualSet := sets.New(actualClusterNames...)

		// Check if sets are equal
		if !expectedSet.Equal(actualSet) {
			return fmt.Errorf("expected MultiKueueClusters %v, got %v", expectedSet.UnsortedList(), actualSet.UnsortedList())
		}

		return nil
	}, eventuallyTimeout, eventuallyInterval).Should(gomega.Succeed())
}

// assertMultiKueueClusterNotExists asserts that a specific MultiKueueCluster does not exist
func assertMultiKueueClusterNotExists(ctx context.Context, client kueueclientset.Interface, clusterName string) {
	ginkgo.By(fmt.Sprintf("Asserting MultiKueueCluster %s does not exist", clusterName))
	gomega.Eventually(func() bool {
		_, err := client.KueueV1beta1().MultiKueueClusters().Get(ctx, clusterName, metav1.GetOptions{})
		return errors.IsNotFound(err)
	}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())
}

// assertAdmissionCheckConditionStatus asserts that AdmissionCheck has condition with specific status
func assertAdmissionCheckConditionStatus(ctx context.Context, client kueueclientset.Interface, acName string, status metav1.ConditionStatus) {
	ginkgo.By(fmt.Sprintf("Asserting AdmissionCheck %s has condition MultiKueueClusterActive=%s", acName, status))
	gomega.Eventually(func() error {
		ac, err := client.KueueV1beta1().AdmissionChecks().Get(ctx, acName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get AdmissionCheck %s: %v", acName, err)
		}

		for _, condition := range ac.Status.Conditions {
			if condition.Type == kueuev1beta1.MultiKueueClusterActive {
				if condition.Status == status {
					return nil
				}
				return fmt.Errorf("expected condition MultiKueueClusterActive=%s, got %s", status, condition.Status)
			}
		}
		return fmt.Errorf("condition MultiKueueClusterActive not found")
	}, eventuallyTimeout, eventuallyInterval).Should(gomega.Succeed())
}

// assertAdmissionCheckConditionTrue asserts that AdmissionCheck has condition set to True
func assertAdmissionCheckConditionTrue(ctx context.Context, client kueueclientset.Interface, acName string) {
	assertAdmissionCheckConditionStatus(ctx, client, acName, metav1.ConditionTrue)
}

// assertAdmissionCheckConditionFalse asserts that AdmissionCheck has condition set to False
func assertAdmissionCheckConditionFalse(ctx context.Context, client kueueclientset.Interface, acName string) {
	assertAdmissionCheckConditionStatus(ctx, client, acName, metav1.ConditionFalse)
}

// Helper function to create kueue-system namespace
func createKueueNamespace(ctx context.Context, hubKubeClient kubernetes.Interface, kueueNamespace string) {
	ginkgo.By("Creating kueue-system namespace")
	_, err := hubKubeClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: kueueNamespace,
		},
	}, metav1.CreateOptions{})
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
}

// Helper function to create a managed cluster
func createManagedCluster(ctx context.Context, hubKubeClient kubernetes.Interface, hubClusterClient clusterv1client.Interface, clusterName string) {
	ginkgo.By(fmt.Sprintf("Creating managed cluster %s", clusterName))
	// Create namespace for the cluster
	_, err := hubKubeClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: clusterName},
	}, metav1.CreateOptions{})
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	// Create ManagedCluster
	_, err = hubClusterClient.ClusterV1().ManagedClusters().Create(ctx, &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: clusterName},
		Spec: clusterv1.ManagedClusterSpec{
			ManagedClusterClientConfigs: []clusterv1.ClientConfig{
				{
					URL: "https://test-cluster:6443",
				},
			},
		},
	}, metav1.CreateOptions{})
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
}

// Helper function to remove a managed cluster
func removeManagedCluster(ctx context.Context, hubClusterClient clusterv1client.Interface, clusterName string) {
	ginkgo.By(fmt.Sprintf("Deleting managed cluster %s", clusterName))
	err := hubClusterClient.ClusterV1().ManagedClusters().Delete(ctx, clusterName, metav1.DeleteOptions{})
	if err != nil {
		ginkgo.GinkgoWriter.Printf("Failed to delete ManagedCluster %s: %v\n", clusterName, err)
	}
}

// Helper function to create a secret generated by managed-serviceaccount controller
func createMsaSecret(ctx context.Context, hubKubeClient kubernetes.Interface, namespace string) {
	ginkgo.By(fmt.Sprintf("Creating secret %s in namespace %s", common.MultiKueueResourceName, namespace))
	_, err := hubKubeClient.CoreV1().Secrets(namespace).Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.MultiKueueResourceName,
			Namespace: namespace,
			Labels: map[string]string{
				"authentication.open-cluster-management.io/is-managed-serviceaccount": "true",
			},
		},
		Data: map[string][]byte{
			"token":  []byte("test-token"),
			"ca.crt": []byte("test-ca-cert"),
		},
	}, metav1.CreateOptions{})
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
}

// Helper function to remove a secret
func removeSecret(ctx context.Context, hubKubeClient kubernetes.Interface, namespace, name string) {
	ginkgo.By(fmt.Sprintf("Deleting secret %s in namespace %s", name, namespace))
	err := hubKubeClient.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		ginkgo.GinkgoWriter.Printf("Failed to delete Secret %s/%s: %v\n", namespace, name, err)
	}
}

// Helper function to create placement and placement decision
func createPlacementWithDecision(ctx context.Context, hubClusterClient clusterv1client.Interface, namespace, placementName string, clusterNames []string) (*clusterv1beta1.Placement, *clusterv1beta1.PlacementDecision) {
	ginkgo.By(fmt.Sprintf("Creating placement %s in namespace %s", placementName, namespace))
	// Create Placement
	placement, err := hubClusterClient.ClusterV1beta1().Placements(namespace).Create(ctx, &clusterv1beta1.Placement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      placementName,
			Namespace: namespace,
		},
	}, metav1.CreateOptions{})
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	// Create PlacementDecision
	pd, err := hubClusterClient.ClusterV1beta1().PlacementDecisions(namespace).Create(ctx, &clusterv1beta1.PlacementDecision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      placementName + "-decision-1",
			Namespace: namespace,
			Labels: map[string]string{
				clusterv1beta1.PlacementLabel:          placementName,
				clusterv1beta1.DecisionGroupIndexLabel: "0",
			},
		},
	}, metav1.CreateOptions{})
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	// Update PlacementDecision status with decisions
	decisions := make([]clusterv1beta1.ClusterDecision, len(clusterNames))
	for i, clusterName := range clusterNames {
		decisions[i] = clusterv1beta1.ClusterDecision{ClusterName: clusterName}
	}
	pd.Status = clusterv1beta1.PlacementDecisionStatus{Decisions: decisions}
	_, err = hubClusterClient.ClusterV1beta1().PlacementDecisions(namespace).UpdateStatus(ctx, pd, metav1.UpdateOptions{})
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	return placement, pd
}

// Helper function to remove placement
func removePlacementWithDecision(ctx context.Context, hubClusterClient clusterv1client.Interface, namespace, placementName string) {
	ginkgo.By(fmt.Sprintf("Deleting placement %s in namespace %s", placementName, namespace))
	err := hubClusterClient.ClusterV1beta1().Placements(namespace).Delete(ctx, placementName, metav1.DeleteOptions{})
	if err != nil {
		ginkgo.GinkgoWriter.Printf("Failed to delete Placement %s/%s: %v\n", namespace, placementName, err)
	}

	err = hubClusterClient.ClusterV1beta1().PlacementDecisions(namespace).Delete(ctx, placementName+"-decision-1", metav1.DeleteOptions{})
	if err != nil {
		ginkgo.GinkgoWriter.Printf("Failed to delete PlacementDecision %s/%s: %v\n", namespace, placementName+"-decision-1", err)
	}
}

// Helper function to create admission check
func createAdmissionCheck(ctx context.Context, hubKueueClient kueueclientset.Interface, acName, placementName string) {
	ginkgo.By(fmt.Sprintf("Creating admission check %s for placement %s", acName, placementName))
	_, err := hubKueueClient.KueueV1beta1().AdmissionChecks().Create(ctx, &kueuev1beta1.AdmissionCheck{
		ObjectMeta: metav1.ObjectMeta{
			Name: acName,
		},
		Spec: kueuev1beta1.AdmissionCheckSpec{
			ControllerName: "open-cluster-management.io/placement",
			Parameters: &kueuev1beta1.AdmissionCheckParametersReference{
				APIGroup: "cluster.open-cluster-management.io",
				Kind:     "Placement",
				Name:     placementName,
			},
		},
	}, metav1.CreateOptions{})
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
}

// Helper function to remove admission check
func removeAdmissionCheck(ctx context.Context, hubKueueClient kueueclientset.Interface, acName string) {
	ginkgo.By(fmt.Sprintf("Deleting admission check %s", acName))
	err := hubKueueClient.KueueV1beta1().AdmissionChecks().Delete(ctx, acName, metav1.DeleteOptions{})
	if err != nil {
		ginkgo.GinkgoWriter.Printf("Failed to delete AdmissionCheck %s: %v\n", acName, err)
	}
}
