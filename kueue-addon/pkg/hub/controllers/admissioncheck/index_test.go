package admissioncheck

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	kueuev1beta2 "sigs.k8s.io/kueue/apis/kueue/v1beta2"
	kueuefake "sigs.k8s.io/kueue/client-go/clientset/versioned/fake"
	kueueinformers "sigs.k8s.io/kueue/client-go/informers/externalversions"

	"open-cluster-management.io/addon-contrib/kueue-addon/pkg/hub/controllers/common"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
)

func TestIndexAdmissionCheckByPlacement(t *testing.T) {
	cases := []struct {
		name         string
		obj          interface{}
		expectedKeys []string
		expectedErr  bool
	}{
		{
			name: "valid admission check",
			obj: &kueuev1beta2.AdmissionCheck{
				Spec: kueuev1beta2.AdmissionCheckSpec{
					ControllerName: common.AdmissionCheckControllerName,
					Parameters: &kueuev1beta2.AdmissionCheckParametersReference{
						Name: "placement1",
					},
				},
			},
			expectedKeys: []string{common.KueueNamespace + "/placement1"},
		},
		{
			name:        "not an admission check",
			obj:         &clusterv1beta1.Placement{},
			expectedErr: true,
		},
		{
			name: "wrong controller name",
			obj: &kueuev1beta2.AdmissionCheck{
				Spec: kueuev1beta2.AdmissionCheckSpec{
					ControllerName: "wrong-controller",
				},
			},
			expectedKeys: []string{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			keys, err := IndexAdmissionCheckByPlacement(c.obj)
			if (err != nil) != c.expectedErr {
				t.Errorf("expected error: %v, got: %v", c.expectedErr, err)
			}
			if len(keys) != len(c.expectedKeys) {
				t.Fatalf("expected %d keys, but got %d", len(c.expectedKeys), len(keys))
			}
			for i, key := range keys {
				if key != c.expectedKeys[i] {
					t.Errorf("expected key %s, but got %s", c.expectedKeys[i], key)
				}
			}
		})
	}
}

func TestAdmissionCheckByPlacementQueueKey(t *testing.T) {
	kueueClient := kueuefake.NewClientset()
	kueueInformerFactory := kueueinformers.NewSharedInformerFactory(kueueClient, 5*time.Minute)
	admissionCheckInformer := kueueInformerFactory.Kueue().V1beta2().AdmissionChecks()
	if err := admissionCheckInformer.Informer().AddIndexers(cache.Indexers{
		AdmissionCheckByPlacement: IndexAdmissionCheckByPlacement,
	}); err != nil {
		t.Fatalf("failed to add indexers: %v", err)
	}

	ac := newAdmissionCheck("ac1", "placement1")
	if err := admissionCheckInformer.Informer().GetStore().Add(ac); err != nil {
		t.Fatalf("failed to add admission check to store: %v", err)
	}

	queueKeyFunc := AdmissionCheckByPlacementQueueKey(admissionCheckInformer)
	placement := newPlacement("placement1", common.KueueNamespace)
	keys := queueKeyFunc(placement)

	if len(keys) != 1 {
		t.Fatalf("expected 1 key, but got %d", len(keys))
	}
	if keys[0] != "ac1" {
		t.Errorf("expected key ac1, but got %s", keys[0])
	}
}

func TestAdmissionCheckByPlacementDecisionQueueKey(t *testing.T) {
	kueueClient := kueuefake.NewClientset()
	kueueInformerFactory := kueueinformers.NewSharedInformerFactory(kueueClient, 5*time.Minute)
	admissionCheckInformer := kueueInformerFactory.Kueue().V1beta2().AdmissionChecks()
	if err := admissionCheckInformer.Informer().AddIndexers(cache.Indexers{
		AdmissionCheckByPlacement: IndexAdmissionCheckByPlacement,
	}); err != nil {
		t.Fatalf("failed to add indexers: %v", err)
	}

	ac := newAdmissionCheck("ac1", "placement1")
	if err := admissionCheckInformer.Informer().GetStore().Add(ac); err != nil {
		t.Fatalf("failed to add admission check to store: %v", err)
	}

	queueKeyFunc := AdmissionCheckByPlacementDecisionQueueKey(admissionCheckInformer)
	placementDecision := &clusterv1beta1.PlacementDecision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: common.KueueNamespace,
			Labels: map[string]string{
				clusterv1beta1.PlacementLabel: "placement1",
			},
		},
	}
	keys := queueKeyFunc(placementDecision)

	if len(keys) != 1 {
		t.Fatalf("expected 1 key, but got %d", len(keys))
	}
	if keys[0] != "ac1" {
		t.Errorf("expected key ac1, but got %s", keys[0])
	}
}
