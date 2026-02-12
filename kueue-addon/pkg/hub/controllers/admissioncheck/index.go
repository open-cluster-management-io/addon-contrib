package admissioncheck

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	kueuev1beta2 "sigs.k8s.io/kueue/apis/kueue/v1beta2"
	kueueinformerv1beta2 "sigs.k8s.io/kueue/client-go/informers/externalversions/kueue/v1beta2"

	"open-cluster-management.io/addon-contrib/kueue-addon/pkg/hub/controllers/common"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
)

const (
	// AdmissionCheckByPlacement is the index name for admission checks by placement
	AdmissionCheckByPlacement = "admissionCheckByPlacement"
)

// IndexAdmissionCheckByPlacement indexes admission checks by their associated placement
func IndexAdmissionCheckByPlacement(obj interface{}) ([]string, error) {
	ac, ok := obj.(*kueuev1beta2.AdmissionCheck)
	if !ok {
		return []string{}, fmt.Errorf("obj %T is not a valid ocm admission check", obj)
	}

	if ac.Spec.ControllerName != common.AdmissionCheckControllerName {
		return []string{}, nil
	}

	placementName := ac.Spec.Parameters.Name
	key := fmt.Sprintf("%s/%s", common.KueueNamespace, placementName)
	return []string{key}, nil
}

// AdmissionCheckByPlacementQueueKey returns a function that generates queue keys for admission checks
// based on placement changes
func AdmissionCheckByPlacementQueueKey(
	aci kueueinformerv1beta2.AdmissionCheckInformer) func(obj runtime.Object) []string {
	return func(obj runtime.Object) []string {
		key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
		if err != nil {
			utilruntime.HandleError(err)
			return []string{}
		}

		objs, err := aci.Informer().GetIndexer().ByIndex(AdmissionCheckByPlacement, key)
		if err != nil {
			utilruntime.HandleError(err)
			return []string{}
		}

		keys := make([]string, 0, len(objs))
		for _, o := range objs {
			ac := o.(*kueuev1beta2.AdmissionCheck)
			klog.V(4).Info("enqueue admission check", "admissionCheck", ac.Name, "placement", key)
			keys = append(keys, ac.Name)
		}

		return keys
	}
}

// AdmissionCheckByPlacementDecisionQueueKey returns a function that generates queue keys for admission checks
// based on placement decision changes
func AdmissionCheckByPlacementDecisionQueueKey(
	aci kueueinformerv1beta2.AdmissionCheckInformer) func(obj runtime.Object) []string {
	return func(obj runtime.Object) []string {
		accessor, _ := meta.Accessor(obj)
		placementName, ok := accessor.GetLabels()[clusterv1beta1.PlacementLabel]
		if !ok {
			return []string{}
		}

		indexKey := fmt.Sprintf("%s/%s", accessor.GetNamespace(), placementName)
		objs, err := aci.Informer().GetIndexer().ByIndex(AdmissionCheckByPlacement, indexKey)
		if err != nil {
			utilruntime.HandleError(err)
			return []string{}
		}

		keys := make([]string, 0, len(objs))
		for _, o := range objs {
			ac := o.(*kueuev1beta2.AdmissionCheck)
			klog.V(4).Info("enqueue admission check",
				"admissionCheck", ac.Name,
				"placementDecision", fmt.Sprintf("%s/%s", accessor.GetNamespace(), accessor.GetName()))
			keys = append(keys, ac.Name)
		}

		return keys
	}
}
