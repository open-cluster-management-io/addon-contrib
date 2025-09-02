package integration

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/rand"
	"open-cluster-management.io/addon-contrib/kueue-addon/pkg/hub/controllers/common"
	"open-cluster-management.io/addon-contrib/kueue-addon/test/helper"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
)

var _ = ginkgo.Describe("Kueue AdmissionCheck Integration", func() {
	var (
		ctx             context.Context
		kueueNamespace  string
		suffix          string
		placementName   string
		acName          string
		cluster1        string
		cluster2        string
		managedClusters []string
	)

	// BeforeEach for all tests
	ginkgo.BeforeEach(func() {
		ctx = context.Background()
		suffix = rand.String(5)
		kueueNamespace = common.KueueNamespace
		placementName = fmt.Sprintf("placement-%s", suffix)
		acName = fmt.Sprintf("test-admissioncheck-%s", suffix)
		cluster1 = fmt.Sprintf("cluster1-%s", suffix)
		cluster2 = fmt.Sprintf("cluster2-%s", suffix)
		managedClusters = []string{cluster1, cluster2}

		// Create kueue-system namespace if it doesn't exist
		_, err := hubKubeClient.CoreV1().Namespaces().Get(ctx, kueueNamespace, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			helper.CreateKueueNamespace(ctx, hubKubeClient, kueueNamespace)
		}

		// Create managed clusters
		helper.CreateManagedCluster(ctx, hubKubeClient, hubClusterClient, cluster1)
		helper.CreateManagedCluster(ctx, hubKubeClient, hubClusterClient, cluster2)

		// Create admission check
		helper.CreateAdmissionCheck(ctx, hubKueueClient, acName, placementName)

		// Assert MultiKueueClusters does not exists since no multikueue secret
		helper.AssertMultiKueueClustersExists(ctx, hubKueueClient, []string{})
	})

	// AfterEach for cleanup
	ginkgo.AfterEach(func() {
		// Clean up admission checks
		helper.RemoveAdmissionCheck(ctx, hubKueueClient, acName)

		// Clean up placements and placement decisions
		helper.RemovePlacementWithDecision(ctx, hubClusterClient, kueueNamespace, placementName)

		// Clean up managed clusters
		for _, clusterName := range managedClusters {
			helper.RemoveSecret(ctx, hubKubeClient, clusterName, common.MultiKueueResourceName)
			helper.RemoveMultiKueueClusters(ctx, hubKueueClient, clusterName)
			helper.RemoveManagedCluster(ctx, hubClusterClient, clusterName)
		}
	})

	ginkgo.Context("AdmissionCheck generate MultiKueueConfig", func() {
		ginkgo.It("should create MultiKueueConfig for clusters in PlacementDecision", func() {
			// Create placement with decision
			helper.CreatePlacementWithDecision(ctx, hubClusterClient, kueueNamespace, placementName, []string{cluster1, cluster2})

			// Assert MultiKueueConfig is created with correct cluster names
			helper.AssertMultiKueueConfigClusters(ctx, hubKueueClient, placementName, []string{cluster1, cluster2})

			// Assert AdmissionCheck status condition is set to True
			helper.AssertAdmissionCheckConditionTrue(ctx, hubKueueClient, acName)
		})

		ginkgo.It("should update MultiKueueConfig when PlacementDecision changes", func() {
			// Create placement with initial decision
			helper.CreatePlacementWithDecision(ctx, hubClusterClient, kueueNamespace, placementName, []string{cluster1})

			// Assert MultiKueueConfig is created with initial cluster
			helper.AssertMultiKueueConfigClusters(ctx, hubKueueClient, placementName, []string{cluster1})

			// Update PlacementDecision to add cluster2
			gomega.Eventually(func() error {
				// Get the latest version of the PlacementDecision
				latestPd, err := hubClusterClient.ClusterV1beta1().PlacementDecisions(kueueNamespace).Get(ctx, placementName+"-decision-1", metav1.GetOptions{})
				if err != nil {
					return err
				}
				latestPd.Status.Decisions = append(latestPd.Status.Decisions, clusterv1beta1.ClusterDecision{ClusterName: cluster2})
				_, err = hubClusterClient.ClusterV1beta1().PlacementDecisions(kueueNamespace).UpdateStatus(ctx, latestPd, metav1.UpdateOptions{})
				return err
			}, 5*time.Second, eventuallyInterval).Should(gomega.Succeed())

			// Assert MultiKueueConfig is updated with both clusters
			helper.AssertMultiKueueConfigClusters(ctx, hubKueueClient, placementName, []string{cluster1, cluster2})

			// Assert condition remains True after update
			helper.AssertAdmissionCheckConditionTrue(ctx, hubKueueClient, acName)
		})

		ginkgo.It("should update MultiKueueConfig when a cluster is removed from PlacementDecision", func() {
			// Create placement with initial decision
			helper.CreatePlacementWithDecision(ctx, hubClusterClient, kueueNamespace, placementName, []string{cluster1, cluster2})

			// Assert MultiKueueConfig is created with both clusters
			helper.AssertMultiKueueConfigClusters(ctx, hubKueueClient, placementName, []string{cluster1, cluster2})

			// Remove cluster2 from PlacementDecision
			gomega.Eventually(func() error {
				// Get the latest version of the PlacementDecision
				latestPd, err := hubClusterClient.ClusterV1beta1().PlacementDecisions(kueueNamespace).Get(ctx, placementName+"-decision-1", metav1.GetOptions{})
				if err != nil {
					return err
				}
				latestPd.Status.Decisions = []clusterv1beta1.ClusterDecision{{ClusterName: cluster1}}
				_, err = hubClusterClient.ClusterV1beta1().PlacementDecisions(kueueNamespace).UpdateStatus(ctx, latestPd, metav1.UpdateOptions{})
				return err
			}, 5*time.Second, eventuallyInterval).Should(gomega.Succeed())

			// Assert MultiKueueConfig is updated with only cluster1
			helper.AssertMultiKueueConfigClusters(ctx, hubKueueClient, placementName, []string{cluster1})

			// Assert condition remains True after cluster removal
			helper.AssertAdmissionCheckConditionTrue(ctx, hubKueueClient, acName)
		})

		ginkgo.It("should delete MultiKueueConfig when no clusters are in PlacementDecision", func() {
			// Create placement with initial decision
			helper.CreatePlacementWithDecision(ctx, hubClusterClient, kueueNamespace, placementName, []string{cluster1})

			// Assert MultiKueueConfig is created with cluster1
			helper.AssertMultiKueueConfigClusters(ctx, hubKueueClient, placementName, []string{cluster1})

			// Remove all clusters from PlacementDecision
			gomega.Eventually(func() error {
				// Get the latest version of the PlacementDecision
				latestPd, err := hubClusterClient.ClusterV1beta1().PlacementDecisions(kueueNamespace).Get(ctx, placementName+"-decision-1", metav1.GetOptions{})
				if err != nil {
					return err
				}
				latestPd.Status.Decisions = []clusterv1beta1.ClusterDecision{}
				_, err = hubClusterClient.ClusterV1beta1().PlacementDecisions(kueueNamespace).UpdateStatus(ctx, latestPd, metav1.UpdateOptions{})
				return err
			}, 5*time.Second, eventuallyInterval).Should(gomega.Succeed())

			// Assert MultiKueueConfig is deleted
			helper.AssertMultiKueueConfigNotExists(ctx, hubKueueClient, placementName)

			// Assert condition is set to False when no clusters are available
			helper.AssertAdmissionCheckConditionFalse(ctx, hubKueueClient, acName)
		})
	})

	ginkgo.Context("ClusterPermission/ManagedServiceAccount integration", func() {
		ginkgo.It("should create ClusterPermission and ManagedServiceAccount for new cluster", func() {
			// Wait for ClusterPermission to be created
			gomega.Eventually(func() bool {
				_, err := hubPermissionClient.ApiV1alpha1().ClusterPermissions(cluster1).Get(ctx, "multikueue", metav1.GetOptions{})
				return err == nil
			}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())

			// Wait for ManagedServiceAccount to be created
			gomega.Eventually(func() bool {
				_, err := hubMSAClient.AuthenticationV1beta1().ManagedServiceAccounts(cluster1).Get(ctx, "multikueue", metav1.GetOptions{})
				return err == nil
			}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())
		})

		ginkgo.It("should clean up resources when cluster has deletion timestamp", func() {
			// Wait for ClusterPermission and ManagedServiceAccount to be created
			gomega.Eventually(func() bool {
				_, err := hubPermissionClient.ApiV1alpha1().ClusterPermissions(cluster1).Get(ctx, "multikueue", metav1.GetOptions{})
				return err == nil
			}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())

			gomega.Eventually(func() bool {
				_, err := hubMSAClient.AuthenticationV1beta1().ManagedServiceAccounts(cluster1).Get(ctx, "multikueue", metav1.GetOptions{})
				return err == nil
			}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())

			// Add a finalizer to prevent immediate deletion
			gomega.Eventually(func() error {
				cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(ctx, cluster1, metav1.GetOptions{})
				if err != nil {
					return err
				}
				cluster.Finalizers = append(cluster.Finalizers, "test-finalizer")
				_, err = hubClusterClient.ClusterV1().ManagedClusters().Update(ctx, cluster, metav1.UpdateOptions{})
				return err
			}, eventuallyTimeout, eventuallyInterval).Should(gomega.Succeed())

			// Delete the ManagedCluster to set deletion timestamp
			helper.RemoveManagedCluster(ctx, hubClusterClient, cluster1)

			// The controller should clean up resources when cluster has deletion timestamp
			gomega.Eventually(func() bool {
				_, err := hubPermissionClient.ApiV1alpha1().ClusterPermissions(cluster1).Get(ctx, "multikueue", metav1.GetOptions{})
				return errors.IsNotFound(err) // Resource should be cleaned up
			}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())

			gomega.Eventually(func() bool {
				_, err := hubMSAClient.AuthenticationV1beta1().ManagedServiceAccounts(cluster1).Get(ctx, "multikueue", metav1.GetOptions{})
				return errors.IsNotFound(err) // Resource should be cleaned up
			}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())

			// Remove the finalizer to allow cluster to be fully deleted
			gomega.Eventually(func() error {
				cluster, err := hubClusterClient.ClusterV1().ManagedClusters().Get(ctx, cluster1, metav1.GetOptions{})
				if err != nil {
					return err
				}
				cluster.Finalizers = []string{}
				_, err = hubClusterClient.ClusterV1().ManagedClusters().Update(ctx, cluster, metav1.UpdateOptions{})
				return err
			}, eventuallyTimeout, eventuallyInterval).Should(gomega.Succeed())
		})
	})

	ginkgo.Context("Secret/MultiKueueClusters copy/gen integration", func() {
		ginkgo.It("should copy ServiceAccount secret to kueue namespace as kubeconfig", func() {
			kubeconfigSecretName := fmt.Sprintf("multikueue-%s", cluster1)

			// Simulate create multikueue secret
			helper.CreateMsaSecret(ctx, hubKubeClient, cluster1)

			// Assert kubeconfig secret is created in kueue namespace
			gomega.Eventually(func() bool {
				_, err := hubKubeClient.CoreV1().Secrets(kueueNamespace).Get(ctx, kubeconfigSecretName, metav1.GetOptions{})
				return err == nil
			}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())

			// Assert MultiKueueClusters exists with correct names
			helper.AssertMultiKueueClustersExists(ctx, hubKueueClient, []string{cluster1})
		})

		ginkgo.It("should update kubeconfig secret when source changes", func() {
			kubeconfigSecretName := fmt.Sprintf("multikueue-%s", cluster1)

			// Simulate create multikueue secret
			helper.CreateMsaSecret(ctx, hubKubeClient, cluster1)

			// Wait for kubeconfig secret to be created
			gomega.Eventually(func() bool {
				_, err := hubKubeClient.CoreV1().Secrets(kueueNamespace).Get(ctx, kubeconfigSecretName, metav1.GetOptions{})
				return err == nil
			}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())

			// Update source secret
			_, err := hubKubeClient.CoreV1().Secrets(cluster1).Update(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: common.MultiKueueResourceName, Namespace: cluster1},
				Data: map[string][]byte{
					"token":  []byte("new-token"),
					"ca.crt": []byte("test-ca-cert"),
				},
			}, metav1.UpdateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			// Assert kubeconfig secret is updated
			gomega.Eventually(func() bool {
				secret, err := hubKubeClient.CoreV1().Secrets(kueueNamespace).Get(ctx, kubeconfigSecretName, metav1.GetOptions{})
				if err != nil {
					return false
				}
				return string(secret.Data["kubeconfig"]) != ""
			}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())

			// Assert MultiKueueClusters exists with correct names
			helper.AssertMultiKueueClustersExists(ctx, hubKueueClient, []string{cluster1})
		})

		ginkgo.It("should delete kubeconfig secret when source is deleted", func() {
			kubeconfigSecretName := fmt.Sprintf("multikueue-%s", cluster1)

			// Simulate create multikueue secret
			helper.CreateMsaSecret(ctx, hubKubeClient, cluster1)

			// Wait for kubeconfig secret to be created
			gomega.Eventually(func() bool {
				_, err := hubKubeClient.CoreV1().Secrets(kueueNamespace).Get(ctx, kubeconfigSecretName, metav1.GetOptions{})
				return err == nil
			}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())

			// Assert MultiKueueClusters exists with correct names
			helper.AssertMultiKueueClustersExists(ctx, hubKueueClient, []string{cluster1})

			// Delete source secret
			err := hubKubeClient.CoreV1().Secrets(cluster1).Delete(ctx, common.MultiKueueResourceName, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			// Assert kubeconfig secret is deleted
			gomega.Eventually(func() bool {
				_, err := hubKubeClient.CoreV1().Secrets(kueueNamespace).Get(ctx, kubeconfigSecretName, metav1.GetOptions{})
				return err != nil
			}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())

			// Assert MultiKueueClusters is deleted
			helper.AssertMultiKueueClustersExists(ctx, hubKueueClient, []string{})
		})
	})
})
