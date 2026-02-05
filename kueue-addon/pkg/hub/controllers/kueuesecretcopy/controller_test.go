package kueuesecretcopy

import (
	"context"
	"testing"
	"time"

	"github.com/openshift/library-go/pkg/operator/events"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	corev1informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/clock"
	kueuev1beta2 "sigs.k8s.io/kueue/apis/kueue/v1beta2"
	kueuefake "sigs.k8s.io/kueue/client-go/clientset/versioned/fake"

	"open-cluster-management.io/addon-contrib/kueue-addon/pkg/hub/controllers/common"
	clusterfake "open-cluster-management.io/api/client/cluster/clientset/versioned/fake"
	clusterinformers "open-cluster-management.io/api/client/cluster/informers/externalversions"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	permissionv1alpha1 "open-cluster-management.io/cluster-permission/api/v1alpha1"
	permissionfake "open-cluster-management.io/cluster-permission/client/clientset/versioned/fake"
	permissioninformers "open-cluster-management.io/cluster-permission/client/informers/externalversions"
)

type testSyncContext struct {
	key      string
	recorder events.Recorder
}

func (t *testSyncContext) Queue() workqueue.RateLimitingInterface { //nolint
	return nil
}

func (t *testSyncContext) QueueKey() string {
	return t.key
}

func (t *testSyncContext) Recorder() events.Recorder {
	return t.recorder
}

func newManagedCluster(name string, url string) *clusterv1.ManagedCluster {
	return &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: clusterv1.ManagedClusterSpec{
			ManagedClusterClientConfigs: []clusterv1.ClientConfig{
				{
					URL: url,
				},
			},
		},
	}
}

func newClusterPermission(clusterName string, ready bool) *permissionv1alpha1.ClusterPermission {
	cp := &permissionv1alpha1.ClusterPermission{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.MultiKueueResourceName,
			Namespace: clusterName,
		},
	}

	if ready {
		cp.Status.Conditions = []metav1.Condition{
			{
				Type:   string(permissionv1alpha1.ConditionTypeAppliedRBACManifestWork),
				Status: metav1.ConditionTrue,
			},
		}
	} else {
		cp.Status.Conditions = []metav1.Condition{
			{
				Type:   string(permissionv1alpha1.ConditionTypeAppliedRBACManifestWork),
				Status: metav1.ConditionFalse,
			},
		}
	}

	return cp
}

func newSourceSecret(namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.MultiKueueResourceName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"token":  []byte("test-token"),
			"ca.crt": []byte("test-ca-cert"),
		},
	}
}

// Helper to check if the expected verb pattern is present in the actions
func hasExpectedSecretVerb(actions []k8stesting.Action, expectedSecretVerb string) bool {
	switch expectedSecretVerb {
	case "create":
		// Look for create action on kubeconfig secret (in kueue-system namespace)
		for _, action := range actions {
			if action.GetVerb() == "create" && action.GetNamespace() == "kueue-system" {
				return true
			}
		}
		return false
	case "delete":
		// Look for delete action on kubeconfig secret (in kueue-system namespace)
		for _, action := range actions {
			if action.GetVerb() == "delete" && action.GetNamespace() == "kueue-system" {
				return true
			}
		}
		return false
	case "delete+create":
		// Look for both delete and create actions on kubeconfig secret
		for i, action := range actions {
			if action.GetVerb() == "delete" && action.GetNamespace() == "kueue-system" {
				if actions[i+1].GetVerb() == "create" && actions[i+1].GetNamespace() == "kueue-system" {
					return true
				}
			}
		}
		return false
	default:
		return false
	}
}

func hasExpectedMKVerb(actions []k8stesting.Action, expectedSecretVerb string) bool {
	switch expectedSecretVerb {
	case "create", "update", "patch":
		if len(actions) == 2 && actions[0].GetVerb() == "get" && actions[1].GetVerb() == expectedSecretVerb {
			return true
		}
		return false
	case "delete":
		if len(actions) == 1 && actions[0].GetVerb() == "delete" {
			return true
		}
		return false
	default:
		return false
	}
}
func TestSync(t *testing.T) {
	cases := []struct {
		name               string
		clusterName        string
		kubeObjects        []runtime.Object
		clusterObjects     []runtime.Object
		kueueObjects       []runtime.Object
		permissionObjects  []runtime.Object
		syncKey            string
		expectedErr        string
		expectedSecretVerb string
		expectedMKVerb     string
	}{
		{
			name:               "create kubeconfig secret and MultiKueueCluster",
			clusterName:        "cluster1",
			kubeObjects:        []runtime.Object{newSourceSecret("cluster1")},
			clusterObjects:     []runtime.Object{newManagedCluster("cluster1", "https://test-server")},
			permissionObjects:  []runtime.Object{newClusterPermission("cluster1", true)},
			syncKey:            "cluster1/multikueue",
			expectedSecretVerb: "create",
			expectedMKVerb:     "create",
		},
		{
			name:        "delete kubeconfig secret and MultiKueueCluster when source secret not found",
			clusterName: "cluster1",
			kubeObjects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      common.GetMultiKueueSecretName("cluster1"),
						Namespace: common.KueueNamespace,
					},
					Data: map[string][]byte{"kubeconfig": []byte("old-config")},
				},
			},
			clusterObjects:    []runtime.Object{newManagedCluster("cluster1", "https://test-server")},
			permissionObjects: []runtime.Object{newClusterPermission("cluster1", true)},
			kueueObjects: []runtime.Object{
				&kueuev1beta2.MultiKueueCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster1"},
				},
			},
			syncKey:            "cluster1/multikueue",
			expectedSecretVerb: "delete",
			expectedMKVerb:     "delete",
		},
		{
			name:               "delete kubeconfig secret and MultiKueueCluster when source secret not found and kubeconfig secret doesn't exist",
			clusterName:        "cluster1",
			kubeObjects:        []runtime.Object{},
			clusterObjects:     []runtime.Object{newManagedCluster("cluster1", "https://test-server")},
			permissionObjects:  []runtime.Object{},
			syncKey:            "cluster1/multikueue",
			expectedSecretVerb: "delete", // Will attempt to delete even if it doesn't exist
			expectedMKVerb:     "delete",
		},
		{
			name:        "delete kubeconfig secret and MultiKueueCluster when cluster permission not found",
			clusterName: "cluster1",
			kubeObjects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      common.GetMultiKueueSecretName("cluster1"),
						Namespace: common.KueueNamespace,
					},
					Data: map[string][]byte{"kubeconfig": []byte("old-config")},
				},
				newSourceSecret("cluster1"),
			},
			clusterObjects:    []runtime.Object{newManagedCluster("cluster1", "https://test-server")},
			permissionObjects: []runtime.Object{},
			kueueObjects: []runtime.Object{
				&kueuev1beta2.MultiKueueCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster1"},
				},
			},
			syncKey:            "cluster1/multikueue",
			expectedSecretVerb: "delete",
			expectedMKVerb:     "delete",
		},
		{
			name:               "managed cluster not found",
			clusterName:        "cluster1",
			kubeObjects:        []runtime.Object{newSourceSecret("cluster1")},
			permissionObjects:  []runtime.Object{newClusterPermission("cluster1", true)},
			syncKey:            "cluster1/multikueue",
			expectedErr:        "failed to get ManagedCluster cluster1: managedcluster.cluster.open-cluster-management.io \"cluster1\" not found",
			expectedSecretVerb: "",
			expectedMKVerb:     "",
		},
		{
			name:        "managed cluster has no url",
			clusterName: "cluster1",
			kubeObjects: []runtime.Object{newSourceSecret("cluster1")},
			clusterObjects: []runtime.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster1"},
				},
			},
			permissionObjects:  []runtime.Object{newClusterPermission("cluster1", true)},
			syncKey:            "cluster1/multikueue",
			expectedErr:        "no client config found for cluster cluster1",
			expectedSecretVerb: "",
			expectedMKVerb:     "",
		},
		{
			name:        "secret missing token",
			clusterName: "cluster1",
			kubeObjects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: common.MultiKueueResourceName, Namespace: "cluster1"},
					Data:       map[string][]byte{"ca.crt": []byte("test-ca-cert")},
				},
			},
			clusterObjects:     []runtime.Object{newManagedCluster("cluster1", "https://test-server")},
			permissionObjects:  []runtime.Object{newClusterPermission("cluster1", true)},
			syncKey:            "cluster1/multikueue",
			expectedErr:        "token not found in secret multikueue",
			expectedSecretVerb: "",
			expectedMKVerb:     "",
		},
		{
			name:        "secret missing ca.crt",
			clusterName: "cluster1",
			kubeObjects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: common.MultiKueueResourceName, Namespace: "cluster1"},
					Data:       map[string][]byte{"token": []byte("test-token")},
				},
			},
			clusterObjects:     []runtime.Object{newManagedCluster("cluster1", "https://test-server")},
			permissionObjects:  []runtime.Object{newClusterPermission("cluster1", true)},
			syncKey:            "cluster1/multikueue",
			expectedErr:        "ca.crt not found in secret multikueue",
			expectedSecretVerb: "",
			expectedMKVerb:     "",
		},
		{
			name:        "secret data empty",
			clusterName: "cluster1",
			kubeObjects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: common.MultiKueueResourceName, Namespace: "cluster1"},
					Data:       map[string][]byte{},
				},
			},
			clusterObjects:     []runtime.Object{newManagedCluster("cluster1", "https://test-server")},
			permissionObjects:  []runtime.Object{newClusterPermission("cluster1", true)},
			syncKey:            "cluster1/multikueue",
			expectedErr:        "token not found in secret multikueue",
			expectedSecretVerb: "",
			expectedMKVerb:     "",
		},
		{
			name:        "managed cluster with multiple client configs",
			clusterName: "cluster1",
			kubeObjects: []runtime.Object{newSourceSecret("cluster1")},
			clusterObjects: []runtime.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster1"},
					Spec: clusterv1.ManagedClusterSpec{
						ManagedClusterClientConfigs: []clusterv1.ClientConfig{
							{URL: "https://first-url"},
							{URL: "https://second-url"},
						},
					},
				},
			},
			permissionObjects:  []runtime.Object{newClusterPermission("cluster1", true)},
			syncKey:            "cluster1/multikueue",
			expectedSecretVerb: "create",
			expectedMKVerb:     "create",
		},
		{
			name:        "target secret already exists but content changes",
			clusterName: "cluster1",
			kubeObjects: []runtime.Object{
				newSourceSecret("cluster1"),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: common.GetMultiKueueSecretName("cluster1"), Namespace: common.KueueNamespace},
					Data:       map[string][]byte{"kubeconfig": []byte("old-config")},
				},
			},
			kueueObjects: []runtime.Object{
				&kueuev1beta2.MultiKueueCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster1"},
					Spec: kueuev1beta2.MultiKueueClusterSpec{
						ClusterSource: kueuev1beta2.ClusterSource{
							KubeConfig: &kueuev1beta2.KubeConfig{
								LocationType: kueuev1beta2.SecretLocationType,
								Location:     "cluster1",
							},
						},
					},
				},
			},
			clusterObjects:     []runtime.Object{newManagedCluster("cluster1", "https://test-server")},
			permissionObjects:  []runtime.Object{newClusterPermission("cluster1", true)},
			syncKey:            "cluster1/multikueue",
			expectedSecretVerb: "delete+create", // resourceapply.ApplySecret delete+create for existing secret
			expectedMKVerb:     "patch",
		},
		{
			name:               "cluster permission not ready (AppliedRBACManifestWork condition false)",
			clusterName:        "cluster1",
			kubeObjects:        []runtime.Object{newSourceSecret("cluster1")},
			clusterObjects:     []runtime.Object{newManagedCluster("cluster1", "https://test-server")},
			permissionObjects:  []runtime.Object{newClusterPermission("cluster1", false)},
			syncKey:            "cluster1/multikueue",
			expectedSecretVerb: "create",
			expectedMKVerb:     "create",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kubeClient := fake.NewClientset(c.kubeObjects...)
			clusterClient := clusterfake.NewSimpleClientset(c.clusterObjects...)
			kueueClient := kueuefake.NewSimpleClientset(c.kueueObjects...) //nolint:staticcheck // SA1019: deprecated but required for kueue v0.16.0
			permissionClient := permissionfake.NewSimpleClientset(c.permissionObjects...)

			secretInformer := corev1informers.NewSecretInformer(kubeClient, metav1.NamespaceAll, 5*time.Minute, nil)
			for _, obj := range c.kubeObjects {
				if err := secretInformer.GetStore().Add(obj); err != nil {
					t.Fatalf("failed to add secret to store: %v", err)
				}
			}

			clusterInformerFactory := clusterinformers.NewSharedInformerFactory(clusterClient, 5*time.Minute)
			clusterInformer := clusterInformerFactory.Cluster().V1().ManagedClusters()
			for _, obj := range c.clusterObjects {
				if err := clusterInformer.Informer().GetStore().Add(obj); err != nil {
					t.Fatalf("failed to add cluster to store: %v", err)
				}
			}

			permissionInformerFactory := permissioninformers.NewSharedInformerFactory(permissionClient, 5*time.Minute)
			permissionInformer := permissionInformerFactory.Api().V1alpha1().ClusterPermissions()
			for _, obj := range c.permissionObjects {
				if err := permissionInformer.Informer().GetStore().Add(obj); err != nil {
					t.Fatalf("failed to add permission to store: %v", err)
				}
			}

			controller := &kueueSecretCopyController{
				kubeClient:       kubeClient,
				kueueClient:      kueueClient,
				clusterLister:    clusterInformer.Lister(),
				permissionLister: permissionInformer.Lister(),
				eventRecorder:    events.NewInMemoryRecorder("test", clock.RealClock{}),
			}

			syncContext := &testSyncContext{
				key:      c.syncKey,
				recorder: events.NewInMemoryRecorder("test", clock.RealClock{}),
			}
			err := controller.sync(context.TODO(), syncContext)

			if c.expectedErr != "" {
				if err == nil || err.Error() != c.expectedErr {
					t.Errorf("expected error %q, but got %v", c.expectedErr, err)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if c.expectedSecretVerb == "" {
				// Check that no create/delete/update actions happened on kubeconfig secrets in kueue-system
				for _, action := range kubeClient.Actions() {
					if action.GetNamespace() == "kueue-system" && (action.GetVerb() == "create" || action.GetVerb() == "delete" || action.GetVerb() == "update") {
						t.Errorf("expected no secret to be created or updated, but got actions: %v", kubeClient.Actions())
						break
					}
				}
			} else {
				if !hasExpectedSecretVerb(kubeClient.Actions(), c.expectedSecretVerb) {
					t.Errorf("expected verb %s for kubeconfig secret, but got actions: %v", c.expectedSecretVerb, kubeClient.Actions())
				}
			}

			if c.expectedMKVerb == "" {
				if len(kueueClient.Actions()) > 0 {
					t.Errorf("expected no MultiKueueCluster to be created or updated, but got actions: %v", kueueClient.Actions())
				}
			} else {
				if !hasExpectedMKVerb(kueueClient.Actions(), c.expectedMKVerb) {
					t.Errorf("expected verb %s for kubeconfig secret, but got actions: %v", c.expectedMKVerb, kueueClient.Actions())
				}
			}

		})
	}
}
