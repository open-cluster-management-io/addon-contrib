package admissioncheck

import (
	"context"
	"fmt"

	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
	kueueclient "sigs.k8s.io/kueue/client-go/clientset/versioned"
	kueueinformerv1beta1 "sigs.k8s.io/kueue/client-go/informers/externalversions/kueue/v1beta1"
	kueuelisterv1beta1 "sigs.k8s.io/kueue/client-go/listers/kueue/v1beta1"

	"open-cluster-management.io/addon-contrib/kueue-addon/pkg/hub/controllers/common"
	clusterclient "open-cluster-management.io/api/client/cluster/clientset/versioned"
	clusterinformerv1beta1 "open-cluster-management.io/api/client/cluster/informers/externalversions/cluster/v1beta1"
	clusterlisterv1beta1 "open-cluster-management.io/api/client/cluster/listers/cluster/v1beta1"
	sdkv1beta1 "open-cluster-management.io/sdk-go/pkg/apis/cluster/v1beta1"
	"open-cluster-management.io/sdk-go/pkg/patcher"

	"open-cluster-management.io/ocm/pkg/common/helpers"
	commonhelpers "open-cluster-management.io/ocm/pkg/common/helpers"
	"open-cluster-management.io/ocm/pkg/common/queue"
)

const (
	admissioncheckControllerName = "AdmissionCheckController"
	admissionCheckFinalizerName  = "kueue-addon.open-cluster-management.io/admissioncheck-cleanup"
)

// AdmissioncheckController manages MultiKueueConfig and MultiKueueCluster resources based on PlacementDecisions.
type admissioncheckController struct {
	clusterClient           clusterclient.Interface
	kueueClient             kueueclient.Interface
	placementLister         clusterlisterv1beta1.PlacementLister
	placementDecisionGetter helpers.PlacementDecisionGetter
	admissioncheckLister    kueuelisterv1beta1.AdmissionCheckLister
	admissioncheckPatcher   patcher.Patcher[*kueuev1beta1.AdmissionCheck, kueuev1beta1.AdmissionCheckSpec, kueuev1beta1.AdmissionCheckStatus]
	eventRecorder           events.Recorder
}

// NewAdmissionCheckController returns a controller that reconciles MultiKueueConfig and MultiKueueCluster resources
// for each AdmissionCheck, based on Placement and PlacementDecision changes.
func NewAdmissionCheckController(
	ctx context.Context,
	clusterClient clusterclient.Interface,
	kueueClient kueueclient.Interface,
	placementInformer clusterinformerv1beta1.PlacementInformer,
	placementDecisionInformer clusterinformerv1beta1.PlacementDecisionInformer,
	admissionCheckInformer kueueinformerv1beta1.AdmissionCheckInformer,
	recorder events.Recorder,
) factory.Controller {
	c := &admissioncheckController{
		clusterClient:           clusterClient,
		kueueClient:             kueueClient,
		placementLister:         placementInformer.Lister(),
		placementDecisionGetter: helpers.PlacementDecisionGetter{Client: placementDecisionInformer.Lister()},
		admissioncheckLister:    admissionCheckInformer.Lister(),
		admissioncheckPatcher:   patcher.NewPatcher[*kueuev1beta1.AdmissionCheck, kueuev1beta1.AdmissionCheckSpec, kueuev1beta1.AdmissionCheckStatus](kueueClient.KueueV1beta1().AdmissionChecks()),
		eventRecorder:           recorder.WithComponentSuffix("admission-check-controller"),
	}

	return factory.New().
		WithFilteredEventsInformersQueueKeysFunc(
			queue.QueueKeyByMetaName,
			func(obj interface{}) bool {
				accessor, _ := meta.Accessor(obj)
				admissionCheck, _ := accessor.(*kueuev1beta1.AdmissionCheck)
				// Filter OCM admission check controller
				return admissionCheck.Spec.ControllerName == common.AdmissionCheckControllerName
			},
			admissionCheckInformer.Informer()).
		WithInformersQueueKeysFunc(
			AdmissionCheckByPlacementQueueKey(admissionCheckInformer), placementInformer.Informer()).
		WithInformersQueueKeysFunc(
			AdmissionCheckByPlacementDecisionQueueKey(admissionCheckInformer), placementDecisionInformer.Informer()).
		WithSync(c.sync).
		ToController(admissioncheckControllerName, recorder)
}

// Sync ensures that MultiKueueConfig and MultiKueueCluster resources match the current PlacementDecision state
// for the given AdmissionCheck. It creates, updates, or deletes resources as needed.
func (c *admissioncheckController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	key := syncCtx.QueueKey()
	logger := klog.FromContext(ctx)
	logger.Info("Reconciling AdmissionCheck", "key", key)

	admissionCheck, err := c.admissioncheckLister.Get(key)
	if errors.IsNotFound(err) {
		logger.Info("AdmissionCheck not found", "key", key)
		return nil
	}
	if err != nil {
		return err
	}

	// Check if AdmissionCheck is being deleted
	if !admissionCheck.DeletionTimestamp.IsZero() {
		logger.Info("AdmissionCheck is being deleted, cleaning up related resources", "key", key)
		return c.cleanupAdmissionCheckResources(ctx, admissionCheck)
	}

	// Add finalizer if not present
	if !commonhelpers.HasFinalizer(admissionCheck.Finalizers, admissionCheckFinalizerName) {
		_, err = c.admissioncheckPatcher.AddFinalizer(ctx, admissionCheck, admissionCheckFinalizerName)
		return err
	}

	// Init placement tracker
	placementName := admissionCheck.Spec.Parameters.Name
	placement, err := c.placementLister.Placements(common.KueueNamespace).Get(placementName)
	if errors.IsNotFound(err) {
		// Placement not found, set condition to False
		newadmissioncheck := admissionCheck.DeepCopy()
		meta.SetStatusCondition(&newadmissioncheck.Status.Conditions, metav1.Condition{
			Type:    kueuev1beta1.MultiKueueClusterActive,
			Status:  metav1.ConditionFalse,
			Reason:  "PlacementNotFound",
			Message: fmt.Sprintf("Placement %s not found", placementName),
		})
		_, err = c.admissioncheckPatcher.PatchStatus(ctx, newadmissioncheck, newadmissioncheck.Status, admissionCheck.Status)
		return fmt.Errorf("placement %s not found, will retry: %v", placementName, err)
	}
	if err != nil {
		// Error getting placement, set condition to False
		newadmissioncheck := admissionCheck.DeepCopy()
		meta.SetStatusCondition(&newadmissioncheck.Status.Conditions, metav1.Condition{
			Type:    kueuev1beta1.MultiKueueClusterActive,
			Status:  metav1.ConditionFalse,
			Reason:  "PlacementError",
			Message: fmt.Sprintf("Failed to get placement %s: %v", placementName, err),
		})
		_, err = c.admissioncheckPatcher.PatchStatus(ctx, newadmissioncheck, newadmissioncheck.Status, admissionCheck.Status)
		return fmt.Errorf("failed to get placement %s: %v", placementName, err)
	}

	// New decision tracker
	pdTracker := sdkv1beta1.NewPlacementDecisionClustersTracker(placement, c.placementDecisionGetter, nil)

	// Refresh and get existing decision clusters
	if err := pdTracker.Refresh(); err != nil {
		// Error refreshing placement decision tracker, set condition to False
		newadmissioncheck := admissionCheck.DeepCopy()
		meta.SetStatusCondition(&newadmissioncheck.Status.Conditions, metav1.Condition{
			Type:    kueuev1beta1.MultiKueueClusterActive,
			Status:  metav1.ConditionFalse,
			Reason:  "PlacementDecisionError",
			Message: fmt.Sprintf("Failed to refresh placement decision tracker: %v", err),
		})
		_, err = c.admissioncheckPatcher.PatchStatus(ctx, newadmissioncheck, newadmissioncheck.Status, admissionCheck.Status)
		return fmt.Errorf("failed to refresh placement decision tracker: %v", err)
	}
	clusters := pdTracker.ExistingClusterGroupsBesides().GetClusters()

	// Build desired MultiKueueConfig and MultiKueueCluster set
	multiKueueConfigName := placementName
	mkconfig := &kueuev1beta1.MultiKueueConfig{
		ObjectMeta: metav1.ObjectMeta{Name: multiKueueConfigName},
		Spec: kueuev1beta1.MultiKueueConfigSpec{
			Clusters: []string{},
		},
	}

	// Record MultiKueueConfig clusters and update MultiKueueCluster
	expectedMKClusterNames := sets.New[string]()
	for cn := range clusters {
		mkclusterName := placementName + "-" + cn
		mkcluster := &kueuev1beta1.MultiKueueCluster{
			ObjectMeta: metav1.ObjectMeta{Name: mkclusterName},
			Spec: kueuev1beta1.MultiKueueClusterSpec{
				KubeConfig: kueuev1beta1.KubeConfig{
					LocationType: kueuev1beta1.SecretLocationType,
					Location:     common.GetMultiKueueSecretName(cn),
				},
			},
		}
		if err := c.createOrUpdateMultiKueueCluster(ctx, mkcluster); err != nil {
			// Error creating/updating MultiKueueCluster, set condition to False
			newadmissioncheck := admissionCheck.DeepCopy()
			meta.SetStatusCondition(&newadmissioncheck.Status.Conditions, metav1.Condition{
				Type:    kueuev1beta1.MultiKueueClusterActive,
				Status:  metav1.ConditionFalse,
				Reason:  "MultiKueueClusterError",
				Message: fmt.Sprintf("Failed to create/update multi kueue cluster %s: %v", mkclusterName, err),
			})
			_, err = c.admissioncheckPatcher.PatchStatus(ctx, newadmissioncheck, newadmissioncheck.Status, admissionCheck.Status)
			return fmt.Errorf("failed to create/update multi kueue cluster %s: %v", mkclusterName, err)
		}
		mkconfig.Spec.Clusters = append(mkconfig.Spec.Clusters, mkclusterName)
		expectedMKClusterNames.Insert(mkclusterName)
	}

	// Clean up MultiKueueClusters
	if err := c.cleanupMultiKueueClusters(ctx, multiKueueConfigName, expectedMKClusterNames); err != nil {
		// Error cleaning up MultiKueueClusters, set condition to False
		newadmissioncheck := admissionCheck.DeepCopy()
		meta.SetStatusCondition(&newadmissioncheck.Status.Conditions, metav1.Condition{
			Type:    kueuev1beta1.MultiKueueClusterActive,
			Status:  metav1.ConditionFalse,
			Reason:  "CleanupError",
			Message: fmt.Sprintf("Failed to cleanup MultiKueueClusters: %v", err),
		})
		_, err = c.admissioncheckPatcher.PatchStatus(ctx, newadmissioncheck, newadmissioncheck.Status, admissionCheck.Status)
		return err
	}

	// Only create/update MultiKueueConfig if there are clusters available
	if len(mkconfig.Spec.Clusters) > 0 {
		if err := c.createOrUpdateMultiKueueConfig(ctx, mkconfig); err != nil {
			// Error creating/updating MultiKueueConfig, set condition to False
			newadmissioncheck := admissionCheck.DeepCopy()
			meta.SetStatusCondition(&newadmissioncheck.Status.Conditions, metav1.Condition{
				Type:    kueuev1beta1.MultiKueueClusterActive,
				Status:  metav1.ConditionFalse,
				Reason:  "MultiKueueConfigError",
				Message: fmt.Sprintf("Failed to create/update multi kueue config %s: %v", mkconfig.Name, err),
			})
			_, err = c.admissioncheckPatcher.PatchStatus(ctx, newadmissioncheck, newadmissioncheck.Status, admissionCheck.Status)
			return fmt.Errorf("failed to create/update multi kueue config %s: %v", mkconfig.Name, err)
		}
	} else {
		// If no clusters, delete the MultiKueueConfig if it exists
		if err := c.deleteMultiKueueConfig(ctx, multiKueueConfigName); err != nil {
			// Error deleting MultiKueueConfig, set condition to False
			newadmissioncheck := admissionCheck.DeepCopy()
			meta.SetStatusCondition(&newadmissioncheck.Status.Conditions, metav1.Condition{
				Type:    kueuev1beta1.MultiKueueClusterActive,
				Status:  metav1.ConditionFalse,
				Reason:  "MultiKueueConfigDeleteError",
				Message: fmt.Sprintf("Failed to delete multi kueue config %s: %v", multiKueueConfigName, err),
			})
			_, err = c.admissioncheckPatcher.PatchStatus(ctx, newadmissioncheck, newadmissioncheck.Status, admissionCheck.Status)
			return fmt.Errorf("failed to delete multi kueue config %s: %v", multiKueueConfigName, err)
		}

		// No clusters available, set condition to False
		newadmissioncheck := admissionCheck.DeepCopy()
		meta.SetStatusCondition(&newadmissioncheck.Status.Conditions, metav1.Condition{
			Type:    kueuev1beta1.MultiKueueClusterActive,
			Status:  metav1.ConditionFalse,
			Reason:  "NoClustersAvailable",
			Message: fmt.Sprintf("No clusters available for placement %s", placementName),
		})
		_, err = c.admissioncheckPatcher.PatchStatus(ctx, newadmissioncheck, newadmissioncheck.Status, admissionCheck.Status)
		return err
	}

	// Update AdmissionCheck status
	newadmissioncheck := admissionCheck.DeepCopy()
	meta.SetStatusCondition(&newadmissioncheck.Status.Conditions, metav1.Condition{
		Type:    kueuev1beta1.MultiKueueClusterActive,
		Status:  metav1.ConditionTrue,
		Reason:  "Active",
		Message: fmt.Sprintf("MultiKueueConfig %s and MultiKueueClusters are generated successfully", placementName),
	})
	_, err = c.admissioncheckPatcher.PatchStatus(ctx, newadmissioncheck, newadmissioncheck.Status, admissionCheck.Status)
	return err
}

// cleanupAdmissionCheckResources cleans up all MultiKueueConfig and MultiKueueCluster resources
// associated with the given AdmissionCheck when it's being deleted.
func (c *admissioncheckController) cleanupAdmissionCheckResources(ctx context.Context, admissionCheck *kueuev1beta1.AdmissionCheck) error {
	placementName := admissionCheck.Spec.Parameters.Name
	logger := klog.FromContext(ctx)

	// Check if finalizer is present
	if !commonhelpers.HasFinalizer(admissionCheck.Finalizers, admissionCheckFinalizerName) {
		logger.Info("No finalizer present, skipping cleanup", "admissionCheck", admissionCheck.Name)
		return nil
	}

	// Delete all MultiKueueClusters associated with this AdmissionCheck
	existingConfig, err := c.kueueClient.KueueV1beta1().MultiKueueConfigs().Get(ctx, placementName, metav1.GetOptions{})
	if err == nil {
		for _, clusterName := range existingConfig.Spec.Clusters {
			if err := c.kueueClient.KueueV1beta1().MultiKueueClusters().Delete(ctx, clusterName, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
				logger.Error(err, "Failed to delete MultiKueueCluster", "clusterName", clusterName)
			}
		}
	} else if !errors.IsNotFound(err) {
		logger.Error(err, "Failed to get MultiKueueConfig for cleanup", "configName", placementName)
	}

	// Delete MultiKueueConfig
	if err := c.deleteMultiKueueConfig(ctx, placementName); err != nil {
		logger.Error(err, "Failed to delete MultiKueueConfig", "configName", placementName)
	}

	logger.Info("Completed cleanup of AdmissionCheck resources", "admissionCheck", admissionCheck.Name, "placement", placementName)

	// Remove finalizer after successful cleanup
	return c.admissioncheckPatcher.RemoveFinalizer(ctx, admissionCheck, admissionCheckFinalizerName)
}

// CreateOrUpdateMultiKueueConfig creates or updates the MultiKueueConfig resource to match the desired cluster list.
func (c *admissioncheckController) createOrUpdateMultiKueueConfig(ctx context.Context, mkconfig *kueuev1beta1.MultiKueueConfig) error {
	oldmkconfig, err := c.kueueClient.KueueV1beta1().MultiKueueConfigs().Get(ctx, mkconfig.Name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err = c.kueueClient.KueueV1beta1().MultiKueueConfigs().Create(ctx, mkconfig, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}

	newmkconfig := oldmkconfig.DeepCopy()
	newmkconfig.Spec.Clusters = mkconfig.Spec.Clusters
	_, err = c.kueueClient.KueueV1beta1().MultiKueueConfigs().Update(ctx, newmkconfig, metav1.UpdateOptions{})
	return err
}

// DeleteMultiKueueConfig deletes the MultiKueueConfig resource if it exists.
func (c *admissioncheckController) deleteMultiKueueConfig(ctx context.Context, configName string) error {
	err := c.kueueClient.KueueV1beta1().MultiKueueConfigs().Delete(ctx, configName, metav1.DeleteOptions{})
	if errors.IsNotFound(err) {
		return nil // Already deleted
	}
	return err
}

// CreateOrUpdateMultiKueueCluster creates or updates a MultiKueueCluster resource for a specific cluster.
func (c *admissioncheckController) createOrUpdateMultiKueueCluster(ctx context.Context, mkc *kueuev1beta1.MultiKueueCluster) error {
	oldmkcluster, err := c.kueueClient.KueueV1beta1().MultiKueueClusters().Get(ctx, mkc.Name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err = c.kueueClient.KueueV1beta1().MultiKueueClusters().Create(ctx, mkc, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}

	newmkc := oldmkcluster.DeepCopy()
	newmkc.Spec.KubeConfig = *mkc.Spec.KubeConfig.DeepCopy()
	_, err = c.kueueClient.KueueV1beta1().MultiKueueClusters().Update(ctx, newmkc, metav1.UpdateOptions{})
	return err
}

// CleanupMultiKueueClusters deletes MultiKueueCluster resources that are no longer referenced in the MultiKueueConfig.
// It ensures that only the expected clusters remain for the given Placement.
func (c *admissioncheckController) cleanupMultiKueueClusters(ctx context.Context, placementName string, expectedMKClusterNames sets.Set[string]) error {
	existingConfig, err := c.kueueClient.KueueV1beta1().MultiKueueConfigs().Get(ctx, placementName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get existing multi kueue config %s: %v", placementName, err)
	}
	for _, oldClusterName := range existingConfig.Spec.Clusters {
		if !expectedMKClusterNames.Has(oldClusterName) {
			if err := c.kueueClient.KueueV1beta1().MultiKueueClusters().Delete(ctx, oldClusterName, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
				return fmt.Errorf("failed to delete multi kueue cluster %s: %v", oldClusterName, err)
			}
		}
	}
	return nil
}
