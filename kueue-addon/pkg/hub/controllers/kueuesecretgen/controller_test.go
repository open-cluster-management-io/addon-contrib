package kueuesecretgen

import (
	"context"
	"testing"
	"time"

	"github.com/openshift/library-go/pkg/operator/events"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/clock"

	"open-cluster-management.io/addon-contrib/kueue-addon/pkg/hub/controllers/common"
	clusterfake "open-cluster-management.io/api/client/cluster/clientset/versioned/fake"
	clusterinformers "open-cluster-management.io/api/client/cluster/informers/externalversions"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	permissionrv1alpha1 "open-cluster-management.io/cluster-permission/api/v1alpha1"
	permissionfake "open-cluster-management.io/cluster-permission/client/clientset/versioned/fake"
	permissioninformer "open-cluster-management.io/cluster-permission/client/informers/externalversions"
	msav1beta1 "open-cluster-management.io/managed-serviceaccount/apis/authentication/v1beta1"
	msafake "open-cluster-management.io/managed-serviceaccount/pkg/generated/clientset/versioned/fake"
	msainformer "open-cluster-management.io/managed-serviceaccount/pkg/generated/informers/externalversions"
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

func newManagedCluster(name string, deleting bool) *clusterv1.ManagedCluster {
	cluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	if deleting {
		now := metav1.Now()
		cluster.DeletionTimestamp = &now
	}
	return cluster
}

func TestSync(t *testing.T) {
	cases := []struct {
		name                   string
		clusterName            string
		existingObjects        []runtime.Object
		permissionObjects      []runtime.Object
		msaObjects             []runtime.Object
		expectedPermissionVerb string
		expectedMSAVerb        string
		expectedErr            string
	}{
		{
			name:                   "create resources for new cluster",
			clusterName:            "cluster1",
			existingObjects:        []runtime.Object{newManagedCluster("cluster1", false)},
			expectedPermissionVerb: "create",
			expectedMSAVerb:        "create",
		},
		{
			name:                   "delete resources for deleting cluster",
			clusterName:            "cluster1",
			existingObjects:        []runtime.Object{newManagedCluster("cluster1", true)},
			expectedPermissionVerb: "delete",
			expectedMSAVerb:        "delete",
		},
		{
			name:        "cluster not found",
			clusterName: "cluster1",
			expectedErr: "", // should not return error
		},
		{
			name:            "update resources if already exist",
			clusterName:     "cluster1",
			existingObjects: []runtime.Object{newManagedCluster("cluster1", false)},
			permissionObjects: []runtime.Object{
				&permissionrv1alpha1.ClusterPermission{
					ObjectMeta: metav1.ObjectMeta{
						Name:      common.MultiKueueResourceName,
						Namespace: "cluster1",
					},
					Spec: permissionrv1alpha1.ClusterPermissionSpec{},
				},
			},
			msaObjects: []runtime.Object{
				&msav1beta1.ManagedServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      common.MultiKueueResourceName,
						Namespace: "cluster1",
					},
					Spec: msav1beta1.ManagedServiceAccountSpec{},
				},
			},
			expectedPermissionVerb: "patch",
			expectedMSAVerb:        "patch",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			clusterClient := clusterfake.NewSimpleClientset(c.existingObjects...)
			permissionClient := permissionfake.NewSimpleClientset(c.permissionObjects...)
			msaClient := msafake.NewSimpleClientset(c.msaObjects...)

			clusterInformerFactory := clusterinformers.NewSharedInformerFactory(clusterClient, 5*time.Minute)
			permissionInformerFactory := permissioninformer.NewSharedInformerFactory(permissionClient, 5*time.Minute)
			msaInformerFactory := msainformer.NewSharedInformerFactory(msaClient, 5*time.Minute)

			clusterInformer := clusterInformerFactory.Cluster().V1().ManagedClusters()
			permissionInformer := permissionInformerFactory.Api().V1alpha1().ClusterPermissions()
			msaInformer := msaInformerFactory.Authentication().V1beta1().ManagedServiceAccounts()

			for _, obj := range c.existingObjects {
				if err := clusterInformer.Informer().GetStore().Add(obj); err != nil {
					t.Fatalf("failed to add cluster to store: %v", err)
				}
			}
			for _, obj := range c.permissionObjects {
				if err := permissionInformer.Informer().GetStore().Add(obj); err != nil {
					t.Fatalf("failed to add permission to store: %v", err)
				}
			}
			for _, obj := range c.msaObjects {
				if err := msaInformer.Informer().GetStore().Add(obj); err != nil {
					t.Fatalf("failed to add msa to store: %v", err)
				}
			}

			controller := &kueueSecretGenController{
				permissionClient: permissionClient,
				msaClient:        msaClient,
				clusterLister:    clusterInformer.Lister(),
				permissionLister: permissionInformer.Lister(),
				msaLister:        msaInformer.Lister(),
				eventRecorder:    events.NewInMemoryRecorder("test", clock.RealClock{}),
			}

			syncContext := &testSyncContext{
				key:      c.clusterName,
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

			if c.expectedPermissionVerb != "" {
				var found bool
				for _, action := range permissionClient.Actions() {
					if action.GetVerb() == c.expectedPermissionVerb && action.GetNamespace() == c.clusterName {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected %s action on permission client in namespace %s, but got actions: %+v", c.expectedPermissionVerb, c.clusterName, permissionClient.Actions())
				}
			}

			if c.expectedMSAVerb != "" {
				var found bool
				for _, action := range msaClient.Actions() {
					if action.GetVerb() == c.expectedMSAVerb && action.GetNamespace() == c.clusterName {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected %s action on msa client in namespace %s, but got actions: %+v", c.expectedMSAVerb, c.clusterName, msaClient.Actions())
				}
			}
		})
	}
}
