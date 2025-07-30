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

	"open-cluster-management.io/addon-contrib/kueue-addon/pkg/hub/controllers/common"
	clusterfake "open-cluster-management.io/api/client/cluster/clientset/versioned/fake"
	clusterinformers "open-cluster-management.io/api/client/cluster/informers/externalversions"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
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
func hasExpectedVerb(actions []k8stesting.Action, expectedVerb string) bool {
	if expectedVerb == "delete+create" {
		if len(actions) > 3 && actions[2].GetVerb() == "delete" && actions[3].GetVerb() == "create" {
			return true
		}
		return false
	}
	if expectedVerb == "delete" {
		if len(actions) == 2 && actions[1].GetVerb() == "delete" {
			return true
		}
		return false
	}
	if len(actions) > 2 && actions[2].GetVerb() == expectedVerb {
		return true
	}
	return false
}

func TestSync(t *testing.T) {
	cases := []struct {
		name           string
		clusterName    string
		kubeObjects    []runtime.Object
		clusterObjects []runtime.Object
		syncKey        string
		expectedErr    string
		expectedVerb   string
	}{
		{
			name:           "create kubeconfig secret",
			clusterName:    "cluster1",
			kubeObjects:    []runtime.Object{newSourceSecret("cluster1")},
			clusterObjects: []runtime.Object{newManagedCluster("cluster1", "https://test-server")},
			syncKey:        "cluster1/multikueue",
			expectedVerb:   "create",
		},
		{
			name:        "delete kubeconfig secret when source secret not found",
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
			clusterObjects: []runtime.Object{newManagedCluster("cluster1", "https://test-server")},
			syncKey:        "cluster1/multikueue",
			expectedVerb:   "delete",
		},
		{
			name:           "delete kubeconfig secret when source secret not found and kubeconfig secret doesn't exist",
			clusterName:    "cluster1",
			kubeObjects:    []runtime.Object{},
			clusterObjects: []runtime.Object{newManagedCluster("cluster1", "https://test-server")},
			syncKey:        "cluster1/multikueue",
			expectedVerb:   "delete", // Will attempt to delete even if it doesn't exist
		},
		{
			name:         "managed cluster not found",
			clusterName:  "cluster1",
			kubeObjects:  []runtime.Object{newSourceSecret("cluster1")},
			syncKey:      "cluster1/multikueue",
			expectedVerb: "",
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
			syncKey:      "cluster1/multikueue",
			expectedErr:  "no client config found for cluster cluster1",
			expectedVerb: "",
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
			clusterObjects: []runtime.Object{newManagedCluster("cluster1", "https://test-server")},
			syncKey:        "cluster1/multikueue",
			expectedErr:    "token not found in secret multikueue",
			expectedVerb:   "",
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
			clusterObjects: []runtime.Object{newManagedCluster("cluster1", "https://test-server")},
			syncKey:        "cluster1/multikueue",
			expectedErr:    "ca.crt not found in secret multikueue",
			expectedVerb:   "",
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
			clusterObjects: []runtime.Object{newManagedCluster("cluster1", "https://test-server")},
			syncKey:        "cluster1/multikueue",
			expectedErr:    "token not found in secret multikueue",
			expectedVerb:   "",
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
			syncKey:      "cluster1/multikueue",
			expectedVerb: "create",
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
			clusterObjects: []runtime.Object{newManagedCluster("cluster1", "https://test-server")},
			syncKey:        "cluster1/multikueue",
			expectedVerb:   "delete+create", // resourceapply.ApplySecret delete+create for existing secret
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kubeClient := fake.NewSimpleClientset(c.kubeObjects...)
			clusterClient := clusterfake.NewSimpleClientset(c.clusterObjects...)

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

			controller := &kueueSecretCopyController{
				kubeClient:    kubeClient,
				clusterLister: clusterInformer.Lister(),
				eventRecorder: events.NewInMemoryRecorder("test", clock.RealClock{}),
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

			if c.expectedVerb == "" {
				if len(kubeClient.Actions()) > 2 {
					t.Errorf("expected no secret to be created or updated, but got actions: %v", kubeClient.Actions())
				}
				return
			}

			if !hasExpectedVerb(kubeClient.Actions(), c.expectedVerb) {
				t.Errorf("expected verb %s for kubeconfig secret, but got actions: %v", c.expectedVerb, kubeClient.Actions())
			}
		})
	}
}
