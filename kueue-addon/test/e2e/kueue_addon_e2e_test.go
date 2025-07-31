package e2e

import (
	"open-cluster-management.io/addon-contrib/kueue-addon/test/helper"
	placementhelpers "open-cluster-management.io/ocm/pkg/placement/helpers/testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
)

var _ = Describe("MultiKueue Scenarios", func() {
	Context("OCM Placement-based MultiKueue Configuration", func() {
		It("Should automatically configure MultiKueue across multiple clusters based on OCM Placement results", func() {
			By("Creating OCM Placement for GPU accelerator labels and GPU score")
			placement := placementhelpers.NewPlacement("kueue-system", "multikueue-config-e2e").WithNOC(1).
				AddPredicate(&metav1.LabelSelector{
					MatchLabels: map[string]string{"accelerator": "nvidia-tesla-t4"},
				}, nil, nil).
				WithPrioritizerPolicy(clusterv1beta1.PrioritizerPolicyModeExact).
				WithScoreCoordinateAddOn("resource-usage-score", "gpuClusterAvailable", 1).
				Build()
			placement.Labels = map[string]string{"test-scenario": "e2e"}
			_, err := hubClusterClient.ClusterV1beta1().Placements("kueue-system").Create(ctx, placement, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("Creating 2 AdmissionChecks for MultiKueue")
			mkAdmissionCheck := &kueuev1beta1.AdmissionCheck{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "multikueue-e2e",
					Labels: map[string]string{"test-scenario": "e2e"},
				},
				Spec: kueuev1beta1.AdmissionCheckSpec{
					ControllerName: "kueue.x-k8s.io/multikueue",
					Parameters: &kueuev1beta1.AdmissionCheckParametersReference{
						APIGroup: "kueue.x-k8s.io",
						Kind:     "MultiKueueConfig",
						Name:     "multikueue-config-e2e",
					},
				},
			}
			_, err = hubKueueClient.KueueV1beta1().AdmissionChecks().Create(ctx, mkAdmissionCheck, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			placementAdmissionCheck := &kueuev1beta1.AdmissionCheck{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "multikueue-config-e2e",
					Labels: map[string]string{"test-scenario": "e2e"},
				},
				Spec: kueuev1beta1.AdmissionCheckSpec{
					ControllerName: "open-cluster-management.io/placement",
					Parameters: &kueuev1beta1.AdmissionCheckParametersReference{
						APIGroup: "cluster.open-cluster-management.io",
						Kind:     "Placement",
						Name:     "multikueue-config-e2e",
					},
				},
			}
			_, err = hubKueueClient.KueueV1beta1().AdmissionChecks().Create(ctx, placementAdmissionCheck, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("Creating ResourceFlavor on the hub cluster")
			resourceFlavor := &kueuev1beta1.ResourceFlavor{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "default-flavor",
					Labels: map[string]string{"test-scenario": "e2e"},
				},
			}
			_, err = hubKueueClient.KueueV1beta1().ResourceFlavors().Create(ctx, resourceFlavor, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("Creating ClusterQueue on the hub cluster")
			clusterQueue := &kueuev1beta1.ClusterQueue{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-queue", Labels: map[string]string{"test-scenario": "e2e"}},
				Spec: kueuev1beta1.ClusterQueueSpec{
					NamespaceSelector: &metav1.LabelSelector{},
					ResourceGroups: []kueuev1beta1.ResourceGroup{{
						CoveredResources: []corev1.ResourceName{"cpu", "memory", "nvidia.com/gpu"},
						Flavors: []kueuev1beta1.FlavorQuotas{{
							Name: "default-flavor",
							Resources: []kueuev1beta1.ResourceQuota{
								{Name: "cpu", NominalQuota: resource.MustParse("9")},
								{Name: "memory", NominalQuota: resource.MustParse("36Gi")},
								{Name: "nvidia.com/gpu", NominalQuota: resource.MustParse("6")},
							},
						}},
					}},
					AdmissionChecks: []string{"multikueue-e2e", "multikueue-config-e2e"},
				},
			}
			_, err = hubKueueClient.KueueV1beta1().ClusterQueues().Create(ctx, clusterQueue, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("Creating LocalQueue on the hub cluster")
			localQueue := &kueuev1beta1.LocalQueue{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "user-queue",
					Namespace: "default",
					Labels:    map[string]string{"test-scenario": "e2e"},
				},
				Spec: kueuev1beta1.LocalQueueSpec{
					ClusterQueue: "cluster-queue",
				},
			}
			_, err = hubKueueClient.KueueV1beta1().LocalQueues("default").Create(ctx, localQueue, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for MultiKueueConfig to create")
			helper.AssertMultiKueueConfigClusters(ctx, hubKueueClient, "multikueue-config-e2e", []string{"cluster2"})

			By("Waiting for MultiKueueClusters to exist")
			helper.AssertMultiKueueClustersExists(ctx, hubKueueClient, "multikueue-config-e2e", []string{"cluster2"})
			helper.AssertMultiKueueClusterActive(ctx, hubKueueClient, "multikueue-config-e2e-cluster2")

			By("Waiting for AdmissionCheck to be active")
			helper.AssertAdmissionCheckConditionTrue(ctx, hubKueueClient, "multikueue-e2e")
			helper.AssertAdmissionCheckConditionTrue(ctx, hubKueueClient, "multikueue-config-e2e")

			By("Waiting for ClusterQueue to be ready")
			helper.AssertClusterQueueReady(ctx, hubKueueClient, "cluster-queue")

			By("Creating a GPU Job")
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "e2e-job-", Namespace: "default",
					Labels: map[string]string{"kueue.x-k8s.io/queue-name": "user-queue", "test-scenario": "e2e"},
				},
				Spec: batchv1.JobSpec{
					Parallelism: &[]int32{1}[0], Completions: &[]int32{1}[0], Suspend: &[]bool{true}[0],
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name: "gpu-job", Image: "gcr.io/k8s-staging-perf-tests/sleep:v0.1.0", Args: []string{"120s"},
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										"cpu": resource.MustParse("1"), "memory": resource.MustParse("200Mi"), "nvidia.com/gpu": resource.MustParse("1"),
									},
									Limits: corev1.ResourceList{
										"cpu": resource.MustParse("1"), "memory": resource.MustParse("200Mi"), "nvidia.com/gpu": resource.MustParse("1"),
									},
								},
							}},
							RestartPolicy: corev1.RestartPolicyNever,
						},
					},
				},
			}
			createdJob, err := hubClient.BatchV1().Jobs("default").Create(ctx, job, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for the workload to be admitted on cluster2")
			helper.AssertWorkloadAdmitted(ctx, hubKueueClient, createdJob.UID, "multikueue-config-e2e-cluster2")

			By("Waiting for MultiKueueConfig to update")
			helper.AssertMultiKueueConfigClusters(ctx, hubKueueClient, "multikueue-config-e2e", []string{"cluster3"})

			By("Waiting for MultiKueueClusters to exist")
			helper.AssertMultiKueueClustersExists(ctx, hubKueueClient, "multikueue-config-e2e", []string{"cluster3"})
			helper.AssertMultiKueueClusterActive(ctx, hubKueueClient, "multikueue-config-e2e-cluster3")

			By("Waiting for AdmissionCheck to be active")
			helper.AssertAdmissionCheckConditionTrue(ctx, hubKueueClient, "multikueue-e2e")
			helper.AssertAdmissionCheckConditionTrue(ctx, hubKueueClient, "multikueue-config-e2e")

			By("Waiting for ClusterQueue to be ready")
			helper.AssertClusterQueueReady(ctx, hubKueueClient, "cluster-queue")

			By("Creating another GPU Job")
			var createdJob2 *batchv1.Job
			createdJob2, err = hubClient.BatchV1().Jobs("default").Create(ctx, job, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for the workload to be admitted on cluster3")
			helper.AssertWorkloadAdmitted(ctx, hubKueueClient, createdJob2.UID, "multikueue-config-e2e-cluster3")
		})
	})
})
