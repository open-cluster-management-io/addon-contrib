package multikueuecluster

import (
	"context"
	"testing"

	"github.com/openshift/library-go/pkg/operator/events"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	clientcmdv1 "k8s.io/client-go/tools/clientcmd/api/v1"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/clock"
	cpv1alpha1 "sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
	cpfake "sigs.k8s.io/cluster-inventory-api/client/clientset/versioned/fake"
	cpinformers "sigs.k8s.io/cluster-inventory-api/client/informers/externalversions"
	kueuev1beta2 "sigs.k8s.io/kueue/apis/kueue/v1beta2"
	kueuefake "sigs.k8s.io/kueue/client-go/clientset/versioned/fake"
	kueueinformers "sigs.k8s.io/kueue/client-go/informers/externalversions"

	"open-cluster-management.io/addon-contrib/kueue-addon/pkg/hub/controllers/common"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	permissionv1alpha1 "open-cluster-management.io/cluster-permission/api/v1alpha1"
	permissionfake "open-cluster-management.io/cluster-permission/client/clientset/versioned/fake"
	permissioninformer "open-cluster-management.io/cluster-permission/client/informers/externalversions"

	cpcontroller "open-cluster-management.io/ocm/pkg/registration/hub/clusterprofile"
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

func newClusterProfile(name string) *cpv1alpha1.ClusterProfile {
	return &cpv1alpha1.ClusterProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: common.KueueNamespace,
			Labels: map[string]string{
				cpv1alpha1.LabelClusterManagerKey: cpcontroller.ClusterProfileManagerName,
				clusterv1.ClusterNameLabelKey:     name,
			},
		},
		Spec: cpv1alpha1.ClusterProfileSpec{
			DisplayName: name,
		},
		Status: cpv1alpha1.ClusterProfileStatus{
			AccessProviders: []cpv1alpha1.AccessProvider{
				{
					Name: cpcontroller.ClusterProfileManagerName,
					Cluster: clientcmdv1.Cluster{
						Server:                   "fake-server-url",
						CertificateAuthorityData: []byte("fake-ca"),
					},
				},
			},
		},
	}
}

func newClusterPermission(namespace string) *permissionv1alpha1.ClusterPermission {
	return &permissionv1alpha1.ClusterPermission{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.MultiKueueResourceName,
			Namespace: namespace,
		},
	}
}

func newSyncedSecret(clusterName string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName + "-" + common.MultiKueueResourceName,
			Namespace: common.KueueNamespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: cpv1alpha1.GroupVersion.String(),
					Kind:       cpv1alpha1.Kind,
					Name:       clusterName,
				},
			},
		},
		Data: map[string][]byte{
			"token": []byte("fake-token"),
		},
	}
}

func newMultiKueueCluster(name string, clusterProfileRef *kueuev1beta2.ClusterProfileReference) *kueuev1beta2.MultiKueueCluster {
	mkc := &kueuev1beta2.MultiKueueCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: kueuev1beta2.MultiKueueClusterSpec{
			ClusterSource: kueuev1beta2.ClusterSource{},
		},
	}
	if clusterProfileRef != nil {
		mkc.Spec.ClusterSource.ClusterProfileRef = clusterProfileRef
	}
	return mkc
}

func TestSync(t *testing.T) {
	cases := []struct {
		name               string
		clusterName        string
		clusterProfileObjs []runtime.Object
		permissionObjs     []runtime.Object
		secretObjs         []runtime.Object
		kueueObjs          []runtime.Object
		expectedMKCVerb    string // create, update, delete, or empty for no-op
		expectedErr        bool
		validateMKC        func(t *testing.T, mkc *kueuev1beta2.MultiKueueCluster)
	}{
		{
			name:               "create MultiKueueCluster when all resources exist",
			clusterName:        "cluster1",
			clusterProfileObjs: []runtime.Object{newClusterProfile("cluster1")},
			permissionObjs:     []runtime.Object{newClusterPermission("cluster1")},
			secretObjs:         []runtime.Object{newSyncedSecret("cluster1")},
			expectedMKCVerb:    "create",
			validateMKC: func(t *testing.T, mkc *kueuev1beta2.MultiKueueCluster) {
				if mkc.Spec.ClusterSource.ClusterProfileRef == nil {
					t.Error("ClusterProfileRef should not be nil")
				}
				if mkc.Spec.ClusterSource.ClusterProfileRef.Name != "cluster1" {
					t.Errorf("Expected ClusterProfileRef.Name 'cluster1', got '%s'", mkc.Spec.ClusterSource.ClusterProfileRef.Name)
				}
			},
		},
		{
			name:               "update MultiKueueCluster when spec is incorrect",
			clusterName:        "cluster1",
			clusterProfileObjs: []runtime.Object{newClusterProfile("cluster1")},
			permissionObjs:     []runtime.Object{newClusterPermission("cluster1")},
			secretObjs:         []runtime.Object{newSyncedSecret("cluster1")},
			kueueObjs: []runtime.Object{
				newMultiKueueCluster("cluster1", &kueuev1beta2.ClusterProfileReference{Name: "wrong-cluster"}),
			},
			expectedMKCVerb: "update",
			validateMKC: func(t *testing.T, mkc *kueuev1beta2.MultiKueueCluster) {
				if mkc.Spec.ClusterSource.ClusterProfileRef == nil {
					t.Error("ClusterProfileRef should not be nil")
				}
				if mkc.Spec.ClusterSource.ClusterProfileRef.Name != "cluster1" {
					t.Errorf("Expected ClusterProfileRef.Name 'cluster1', got '%s'", mkc.Spec.ClusterSource.ClusterProfileRef.Name)
				}
			},
		},
		{
			name:            "delete MultiKueueCluster when ClusterProfile missing",
			clusterName:     "cluster1",
			permissionObjs:  []runtime.Object{newClusterPermission("cluster1")},
			secretObjs:      []runtime.Object{newSyncedSecret("cluster1")},
			kueueObjs:       []runtime.Object{newMultiKueueCluster("cluster1", &kueuev1beta2.ClusterProfileReference{Name: "cluster1"})},
			expectedMKCVerb: "delete",
		},
		{
			name:               "delete MultiKueueCluster when ClusterPermission missing",
			clusterName:        "cluster1",
			clusterProfileObjs: []runtime.Object{newClusterProfile("cluster1")},
			secretObjs:         []runtime.Object{newSyncedSecret("cluster1")},
			kueueObjs:          []runtime.Object{newMultiKueueCluster("cluster1", &kueuev1beta2.ClusterProfileReference{Name: "cluster1"})},
			expectedMKCVerb:    "delete",
		},
		{
			name:               "delete MultiKueueCluster when synced secret missing",
			clusterName:        "cluster1",
			clusterProfileObjs: []runtime.Object{newClusterProfile("cluster1")},
			permissionObjs:     []runtime.Object{newClusterPermission("cluster1")},
			kueueObjs:          []runtime.Object{newMultiKueueCluster("cluster1", &kueuev1beta2.ClusterProfileReference{Name: "cluster1"})},
			expectedMKCVerb:    "delete",
		},
		{
			name:               "no-op when MultiKueueCluster is correct",
			clusterName:        "cluster1",
			clusterProfileObjs: []runtime.Object{newClusterProfile("cluster1")},
			permissionObjs:     []runtime.Object{newClusterPermission("cluster1")},
			secretObjs:         []runtime.Object{newSyncedSecret("cluster1")},
			kueueObjs:          []runtime.Object{newMultiKueueCluster("cluster1", &kueuev1beta2.ClusterProfileReference{Name: "cluster1"})},
			expectedMKCVerb:    "", // no change expected
		},
		{
			name:               "skip creation when secret missing",
			clusterName:        "cluster1",
			clusterProfileObjs: []runtime.Object{newClusterProfile("cluster1")},
			permissionObjs:     []runtime.Object{newClusterPermission("cluster1")},
			expectedMKCVerb:    "delete", // Should attempt cleanup when secret is missing
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.TODO()

			// Create fake clients
			cpClient := cpfake.NewSimpleClientset(tc.clusterProfileObjs...)
			permissionClient := permissionfake.NewSimpleClientset(tc.permissionObjs...)
			kubeClient := k8sfake.NewClientset(tc.secretObjs...)
			kueueClient := kueuefake.NewSimpleClientset(tc.kueueObjs...) //nolint:staticcheck // SA1019: deprecated but required for kueue v0.16.0

			// Create informers
			cpInformers := cpinformers.NewSharedInformerFactory(cpClient, 0)
			permissionInformers := permissioninformer.NewSharedInformerFactory(permissionClient, 0)
			kubeInformers := kubefake.NewSharedInformerFactory(kubeClient, 0)
			kueueInformers := kueueinformers.NewSharedInformerFactory(kueueClient, 0)

			// Get specific informers
			cpInformer := cpInformers.Apis().V1alpha1().ClusterProfiles()
			permissionInformer := permissionInformers.Api().V1alpha1().ClusterPermissions()
			secretInformer := kubeInformers.Core().V1().Secrets()
			mkclusterInformer := kueueInformers.Kueue().V1beta2().MultiKueueClusters()

			// Manually add objects to informer stores
			for _, obj := range tc.clusterProfileObjs {
				if err := cpInformer.Informer().GetStore().Add(obj); err != nil {
					t.Fatalf("failed to add ClusterProfile to store: %v", err)
				}
			}
			for _, obj := range tc.permissionObjs {
				if err := permissionInformer.Informer().GetStore().Add(obj); err != nil {
					t.Fatalf("failed to add ClusterPermission to store: %v", err)
				}
			}
			for _, obj := range tc.secretObjs {
				if err := secretInformer.Informer().GetStore().Add(obj); err != nil {
					t.Fatalf("failed to add Secret to store: %v", err)
				}
			}
			for _, obj := range tc.kueueObjs {
				if err := mkclusterInformer.Informer().GetStore().Add(obj); err != nil {
					t.Fatalf("failed to add MultiKueueCluster to store: %v", err)
				}
			}

			// Create controller
			controller := &multiKueueClusterController{
				kueueClient:          kueueClient,
				clusterProfileLister: cpInformer.Lister(),
				permissionLister:     permissionInformer.Lister(),
				secretInformer:       secretInformer,
				eventRecorder:        events.NewInMemoryRecorder("test", clock.RealClock{}),
			}

			// Run sync
			syncCtx := &testSyncContext{
				key:      tc.clusterName,
				recorder: events.NewInMemoryRecorder("test", clock.RealClock{}),
			}

			err := controller.sync(ctx, syncCtx)

			// Check error expectation
			if tc.expectedErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.expectedErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Verify expected verb
			if tc.expectedMKCVerb != "" {
				actions := kueueClient.Actions()
				found := false
				for _, action := range actions {
					if action.GetVerb() == tc.expectedMKCVerb && action.GetResource().Resource == "multikueueclusters" {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected MultiKueueCluster %s action, but didn't find it. Actions: %v", tc.expectedMKCVerb, actions)
				}
			}

			// Run custom validation if provided
			if tc.validateMKC != nil && (tc.expectedMKCVerb == "create" || tc.expectedMKCVerb == "update") {
				mkc, err := kueueClient.KueueV1beta2().MultiKueueClusters().Get(ctx, tc.clusterName, metav1.GetOptions{})
				if err != nil {
					t.Fatalf("Failed to get MultiKueueCluster: %v", err)
				}
				tc.validateMKC(t, mkc)
			}
		})
	}
}

func TestNeedsUpdate(t *testing.T) {
	cases := []struct {
		name     string
		existing *kueuev1beta2.MultiKueueCluster
		desired  kueuev1beta2.MultiKueueClusterSpec
		expected bool
	}{
		{
			name:     "both nil ClusterProfileRef - no update needed",
			existing: newMultiKueueCluster("test", nil),
			desired: kueuev1beta2.MultiKueueClusterSpec{
				ClusterSource: kueuev1beta2.ClusterSource{},
			},
			expected: false,
		},
		{
			name:     "existing nil, desired has ref - update needed",
			existing: newMultiKueueCluster("test", nil),
			desired: kueuev1beta2.MultiKueueClusterSpec{
				ClusterSource: kueuev1beta2.ClusterSource{
					ClusterProfileRef: &kueuev1beta2.ClusterProfileReference{Name: "cluster1"},
				},
			},
			expected: true,
		},
		{
			name:     "existing has ref, desired nil - update needed",
			existing: newMultiKueueCluster("test", &kueuev1beta2.ClusterProfileReference{Name: "cluster1"}),
			desired: kueuev1beta2.MultiKueueClusterSpec{
				ClusterSource: kueuev1beta2.ClusterSource{},
			},
			expected: true,
		},
		{
			name:     "same ClusterProfileRef - no update needed",
			existing: newMultiKueueCluster("test", &kueuev1beta2.ClusterProfileReference{Name: "cluster1"}),
			desired: kueuev1beta2.MultiKueueClusterSpec{
				ClusterSource: kueuev1beta2.ClusterSource{
					ClusterProfileRef: &kueuev1beta2.ClusterProfileReference{Name: "cluster1"},
				},
			},
			expected: false,
		},
		{
			name:     "different ClusterProfileRef - update needed",
			existing: newMultiKueueCluster("test", &kueuev1beta2.ClusterProfileReference{Name: "cluster1"}),
			desired: kueuev1beta2.MultiKueueClusterSpec{
				ClusterSource: kueuev1beta2.ClusterSource{
					ClusterProfileRef: &kueuev1beta2.ClusterProfileReference{Name: "cluster2"},
				},
			},
			expected: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := needsUpdate(tc.existing, tc.desired)
			if result != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestGetClusterProfile(t *testing.T) {
	cases := []struct {
		name               string
		clusterName        string
		clusterProfileObjs []runtime.Object
		expectError        bool
		expectNil          bool
	}{
		{
			name:               "valid ClusterProfile",
			clusterName:        "cluster1",
			clusterProfileObjs: []runtime.Object{newClusterProfile("cluster1")},
			expectError:        false,
			expectNil:          false,
		},
		{
			name:        "ClusterProfile not found",
			clusterName: "cluster1",
			expectError: false,
			expectNil:   true,
		},
		{
			name:        "ClusterProfile with wrong manager label",
			clusterName: "cluster1",
			clusterProfileObjs: []runtime.Object{
				&cpv1alpha1.ClusterProfile{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cluster1",
						Namespace: common.KueueNamespace,
						Labels: map[string]string{
							cpv1alpha1.LabelClusterManagerKey: "wrong-manager",
							clusterv1.ClusterNameLabelKey:     "cluster1",
						},
					},
				},
			},
			expectError: true,
			expectNil:   false,
		},
		{
			name:        "ClusterProfile with wrong cluster name label",
			clusterName: "cluster1",
			clusterProfileObjs: []runtime.Object{
				&cpv1alpha1.ClusterProfile{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cluster1",
						Namespace: common.KueueNamespace,
						Labels: map[string]string{
							cpv1alpha1.LabelClusterManagerKey: cpcontroller.ClusterProfileManagerName,
							clusterv1.ClusterNameLabelKey:     "wrong-cluster",
						},
					},
				},
			},
			expectError: true,
			expectNil:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.TODO()

			cpClient := cpfake.NewSimpleClientset(tc.clusterProfileObjs...)
			cpInformers := cpinformers.NewSharedInformerFactory(cpClient, 0)
			cpInformer := cpInformers.Apis().V1alpha1().ClusterProfiles()

			// Manually add objects to informer store
			for _, obj := range tc.clusterProfileObjs {
				if err := cpInformer.Informer().GetStore().Add(obj); err != nil {
					t.Fatalf("failed to add ClusterProfile to store: %v", err)
				}
			}

			controller := &multiKueueClusterController{
				clusterProfileLister: cpInformer.Lister(),
			}

			cp, err := controller.getClusterProfile(ctx, tc.clusterName)

			if tc.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if tc.expectNil && cp != nil {
				t.Error("Expected nil ClusterProfile but got one")
			}
			if !tc.expectNil && !tc.expectError && cp == nil {
				t.Error("Expected ClusterProfile but got nil")
			}
		})
	}
}
