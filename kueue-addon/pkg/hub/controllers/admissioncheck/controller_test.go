package admissioncheck

import (
	"context"
	"testing"
	"time"

	"github.com/openshift/library-go/pkg/operator/events"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/clock"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
	kueuefake "sigs.k8s.io/kueue/client-go/clientset/versioned/fake"
	kueueinformers "sigs.k8s.io/kueue/client-go/informers/externalversions"

	"open-cluster-management.io/addon-contrib/kueue-addon/pkg/hub/controllers/common"
	clusterfake "open-cluster-management.io/api/client/cluster/clientset/versioned/fake"
	clusterinformers "open-cluster-management.io/api/client/cluster/informers/externalversions"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	"open-cluster-management.io/sdk-go/pkg/patcher"

	"open-cluster-management.io/ocm/pkg/common/helpers"
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

func newPlacement(name, namespace string) *clusterv1beta1.Placement {
	return &clusterv1beta1.Placement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func newPlacementDecision(name, namespace, placementName string, clusterNames ...string) *clusterv1beta1.PlacementDecision {
	decisions := []clusterv1beta1.ClusterDecision{}
	for _, clusterName := range clusterNames {
		decisions = append(decisions, clusterv1beta1.ClusterDecision{ClusterName: clusterName})
	}
	return &clusterv1beta1.PlacementDecision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				clusterv1beta1.PlacementLabel:          placementName,
				clusterv1beta1.DecisionGroupIndexLabel: "0",
			},
		},
		Status: clusterv1beta1.PlacementDecisionStatus{
			Decisions: decisions,
		},
	}
}

func newAdmissionCheck(name, placementName string) *kueuev1beta1.AdmissionCheck {
	return &kueuev1beta1.AdmissionCheck{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Finalizers: []string{
				admissionCheckFinalizerName,
			},
		},
		Spec: kueuev1beta1.AdmissionCheckSpec{
			ControllerName: common.AdmissionCheckControllerName,
			Parameters: &kueuev1beta1.AdmissionCheckParametersReference{
				APIGroup: "cluster.open-cluster-management.io",
				Kind:     "Placement",
				Name:     placementName,
			},
		},
	}
}

func TestSync(t *testing.T) {
	cases := []struct {
		name                     string
		admissionCheckName       string
		clusterObjects           []runtime.Object
		kueueObjects             []runtime.Object
		expectedMKClusters       int
		expectedMKConfigClusters int
		expectedStatusCondition  bool
		expectedErr              string
		preExistingMKClusters    []runtime.Object
	}{
		{
			name:               "create multikueue resources",
			admissionCheckName: "ac1",
			clusterObjects: []runtime.Object{
				newPlacement("placement1", common.KueueNamespace),
				newPlacementDecision("placement1-decision-1", common.KueueNamespace, "placement1", "cluster1", "cluster2"),
			},
			kueueObjects: []runtime.Object{
				newAdmissionCheck("ac1", "placement1"),
			},
			expectedMKClusters:       2,
			expectedMKConfigClusters: 2,
			expectedStatusCondition:  true,
		},
		{
			name:               "no placement decision",
			admissionCheckName: "ac1",
			clusterObjects: []runtime.Object{
				newPlacement("placement1", common.KueueNamespace),
			},
			kueueObjects: []runtime.Object{
				newAdmissionCheck("ac1", "placement1"),
			},
			expectedMKClusters:       0,
			expectedMKConfigClusters: 0,
			expectedStatusCondition:  false,
		},
		{
			name:               "remove multikueuecluster when cluster removed",
			admissionCheckName: "ac1",
			clusterObjects: []runtime.Object{
				newPlacement("placement1", common.KueueNamespace),
				newPlacementDecision("placement1-decision-1", common.KueueNamespace, "placement1", "cluster1"), // only cluster1 remains
			},
			kueueObjects: []runtime.Object{
				newAdmissionCheck("ac1", "placement1"),
				&kueuev1beta1.MultiKueueCluster{ObjectMeta: metav1.ObjectMeta{Name: "placement1-cluster1"}},
				&kueuev1beta1.MultiKueueCluster{ObjectMeta: metav1.ObjectMeta{Name: "placement1-cluster2"}}, // should be deleted
				&kueuev1beta1.MultiKueueConfig{
					ObjectMeta: metav1.ObjectMeta{Name: "placement1"},
					Spec:       kueuev1beta1.MultiKueueConfigSpec{Clusters: []string{"placement1-cluster1", "placement1-cluster2"}},
				},
			},
			expectedMKClusters:       1,
			expectedMKConfigClusters: 1,
			expectedStatusCondition:  true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			clusterClient := clusterfake.NewSimpleClientset(c.clusterObjects...)
			kueueClient := kueuefake.NewSimpleClientset(c.kueueObjects...)

			clusterInformerFactory := clusterinformers.NewSharedInformerFactory(clusterClient, 5*time.Minute)
			kueueInformerFactory := kueueinformers.NewSharedInformerFactory(kueueClient, 5*time.Minute)

			placementInformer := clusterInformerFactory.Cluster().V1beta1().Placements()
			placementDecisionInformer := clusterInformerFactory.Cluster().V1beta1().PlacementDecisions()
			admissionCheckInformer := kueueInformerFactory.Kueue().V1beta1().AdmissionChecks()
			if err := admissionCheckInformer.Informer().AddIndexers(cache.Indexers{
				AdmissionCheckByPlacement: IndexAdmissionCheckByPlacement,
			}); err != nil {
				t.Fatalf("failed to add indexers: %v", err)
			}

			for _, obj := range c.clusterObjects {
				switch o := obj.(type) {
				case *clusterv1beta1.Placement:
					if err := placementInformer.Informer().GetStore().Add(o); err != nil {
						t.Fatalf("failed to add placement to store: %v", err)
					}
				case *clusterv1beta1.PlacementDecision:
					if err := placementDecisionInformer.Informer().GetStore().Add(o); err != nil {
						t.Fatalf("failed to add placement decision to store: %v", err)
					}
				}
			}
			for _, obj := range c.kueueObjects {
				if ac, ok := obj.(*kueuev1beta1.AdmissionCheck); ok {
					if err := admissionCheckInformer.Informer().GetStore().Add(ac); err != nil {
						t.Fatalf("failed to add admission check to store: %v", err)
					}
				}
			}

			controller := &admissioncheckController{
				clusterClient:           clusterClient,
				kueueClient:             kueueClient,
				placementLister:         placementInformer.Lister(),
				placementDecisionGetter: helpers.PlacementDecisionGetter{Client: placementDecisionInformer.Lister()},
				admissioncheckLister:    admissionCheckInformer.Lister(),
				admissioncheckPatcher:   patcher.NewPatcher[*kueuev1beta1.AdmissionCheck, kueuev1beta1.AdmissionCheckSpec, kueuev1beta1.AdmissionCheckStatus](kueueClient.KueueV1beta1().AdmissionChecks()),
				eventRecorder:           events.NewInMemoryRecorder("test", clock.RealClock{}),
			}

			syncContext := &testSyncContext{
				key:      c.admissionCheckName,
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

			mkclusters, _ := kueueClient.KueueV1beta1().MultiKueueClusters().List(context.TODO(), metav1.ListOptions{})
			if len(mkclusters.Items) != c.expectedMKClusters {
				t.Errorf("expected %d multikueue clusters, but got %d", c.expectedMKClusters, len(mkclusters.Items))
			}

			mkconfigs, _ := kueueClient.KueueV1beta1().MultiKueueConfigs().List(context.TODO(), metav1.ListOptions{})
			if len(mkconfigs.Items) > 0 {
				if len(mkconfigs.Items[0].Spec.Clusters) != c.expectedMKConfigClusters {
					t.Errorf("expected %d clusters in multikueue config, but got %d", c.expectedMKConfigClusters, len(mkconfigs.Items[0].Spec.Clusters))
				}
			} else if c.expectedMKConfigClusters > 0 {
				t.Errorf("expected multikueue config to be created, but it was not")
			}

			if c.expectedStatusCondition {
				ac, _ := kueueClient.KueueV1beta1().AdmissionChecks().Get(context.TODO(), c.admissionCheckName, metav1.GetOptions{})
				if len(ac.Status.Conditions) == 0 {
					t.Errorf("expected admission check status condition to be updated, but it was not")
				}
			}
		})
	}
}
