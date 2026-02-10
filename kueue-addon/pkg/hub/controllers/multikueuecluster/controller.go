package multikueuecluster

import (
	"context"
	"fmt"

	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	corev1informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/klog/v2"
	cpv1alpha1 "sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
	cpinformers "sigs.k8s.io/cluster-inventory-api/client/informers/externalversions/apis/v1alpha1"
	cplisterv1alpha1 "sigs.k8s.io/cluster-inventory-api/client/listers/apis/v1alpha1"
	kueuev1beta2 "sigs.k8s.io/kueue/apis/kueue/v1beta2"
	kueueclient "sigs.k8s.io/kueue/client-go/clientset/versioned"
	kueueinformerv1beta2 "sigs.k8s.io/kueue/client-go/informers/externalversions/kueue/v1beta2"

	"open-cluster-management.io/addon-contrib/kueue-addon/pkg/hub/controllers/common"
	v1 "open-cluster-management.io/api/cluster/v1"
	permissioninformer "open-cluster-management.io/cluster-permission/client/informers/externalversions/api/v1alpha1"
	permissionlisterv1alpha1 "open-cluster-management.io/cluster-permission/client/listers/api/v1alpha1"

	"open-cluster-management.io/ocm/pkg/common/queue"
	cpcontroller "open-cluster-management.io/ocm/pkg/registration/hub/clusterprofile"
)

// multiKueueClusterController reconciles MultiKueueCluster resources based on ClusterProfile objects
type multiKueueClusterController struct {
	kueueClient          kueueclient.Interface
	clusterProfileLister cplisterv1alpha1.ClusterProfileLister
	permissionLister     permissionlisterv1alpha1.ClusterPermissionLister
	secretInformer       corev1informers.SecretInformer
	eventRecorder        events.Recorder
}

// NewMultiKueueClusterController creates a new controller that manages MultiKueueCluster resources
func NewMultiKueueClusterController(
	kueueClient kueueclient.Interface,
	clusterProfileInformer cpinformers.ClusterProfileInformer,
	permissionInformer permissioninformer.ClusterPermissionInformer,
	secretInformer corev1informers.SecretInformer,
	multiKueueClusterInformer kueueinformerv1beta2.MultiKueueClusterInformer,
	recorder events.Recorder) factory.Controller {
	c := &multiKueueClusterController{
		kueueClient:          kueueClient,
		clusterProfileLister: clusterProfileInformer.Lister(),
		permissionLister:     permissionInformer.Lister(),
		secretInformer:       secretInformer,
		eventRecorder:        recorder.WithComponentSuffix("multikueuecluster-controller"),
	}

	return factory.New().
		WithInformersQueueKeysFunc(
			func(obj runtime.Object) []string {
				accessor, _ := meta.Accessor(obj)
				owners := accessor.GetOwnerReferences()
				if len(owners) != 1 {
					return []string{}
				}
				cpowner := owners[0]
				if cpowner.Kind != cpv1alpha1.Kind {
					return []string{}
				}
				// Queue key is the name of the secret's owner (clusterprofile)
				return []string{owners[0].Name}
			},
			secretInformer.Informer()).
		WithInformersQueueKeysFunc(
			queue.QueueKeyByMetaName,
			multiKueueClusterInformer.Informer()).
		WithInformersQueueKeysFunc(
			queue.QueueKeyByMetaNamespace,
			permissionInformer.Informer()).
		WithInformersQueueKeysFunc(
			queue.QueueKeyByMetaName,
			clusterProfileInformer.Informer()).
		WithSync(c.sync).
		ToController("multiKueueClusterController", recorder)
}

func (c *multiKueueClusterController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	clusterName := syncCtx.QueueKey()
	logger := klog.FromContext(ctx)
	logger.Info("Reconciling MultiKueueCluster", "cluster", clusterName)

	// Step 1: Check if cleanup is needed
	shouldCleanup, err := c.shouldCleanupCluster(ctx, clusterName)
	if err != nil {
		return fmt.Errorf("failed to check cleanup conditions for cluster %s: %v", clusterName, err)
	}

	if shouldCleanup {
		logger.V(4).Info("Cleanup conditions met, deleting MultiKueueCluster", "cluster", clusterName)
		return c.cleanupCluster(ctx, clusterName)
	}

	// Step 2: Verify ClusterProfile exists and get it
	clusterProfile, err := c.getClusterProfile(ctx, clusterName)
	if err != nil {
		return fmt.Errorf("failed to get ClusterProfile for cluster %s: %v", clusterName, err)
	}
	if clusterProfile == nil {
		// ClusterProfile doesn't exist, nothing to do
		logger.V(4).Info("ClusterProfile not found, skipping reconciliation", "cluster", clusterName)
		return nil
	}

	// Step 3: Verify synced secret exists
	_, err = c.secretInformer.Lister().Secrets(common.KueueNamespace).Get(fmt.Sprintf("%s-%s", clusterName, common.MultiKueueResourceName))
	if err != nil {
		return fmt.Errorf("failed to get synced secret for cluster %s: %v", clusterName, err)
	}

	// Step 4: Create/update MultiKueueCluster
	return c.createOrUpdateMultiKueueCluster(ctx, clusterName)
}

// shouldCleanupCluster checks if the MultiKueueCluster should be deleted
// Returns true if any of the following conditions are met:
// - ClusterProfile doesn't exist
// - ClusterPermission doesn't exist
// - Synced secret doesn't exist
func (c *multiKueueClusterController) shouldCleanupCluster(ctx context.Context, clusterName string) (bool, error) {
	logger := klog.FromContext(ctx)

	// Check ClusterProfile
	_, err := c.clusterProfileLister.ClusterProfiles(common.KueueNamespace).Get(clusterName)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.V(4).Info("ClusterProfile not found, cleanup needed", "cluster", clusterName)
			return true, nil
		}
		return false, err
	}

	// Check ClusterPermission
	_, err = c.permissionLister.ClusterPermissions(clusterName).Get(common.MultiKueueResourceName)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.V(4).Info("ClusterPermission not found, cleanup needed", "cluster", clusterName)
			return true, nil
		}
		return false, err
	}

	// Check synced secret
	_, err = c.secretInformer.Lister().Secrets(common.KueueNamespace).Get(fmt.Sprintf("%s-%s", clusterName, common.MultiKueueResourceName))
	if err != nil {
		if errors.IsNotFound(err) {
			logger.V(4).Info("Synced secret not found, cleanup needed", "cluster", clusterName)
			return true, nil
		}
		return false, err
	}

	return false, nil
}

// getClusterProfile retrieves and validates the ClusterProfile for the given cluster
func (c *multiKueueClusterController) getClusterProfile(ctx context.Context, clusterName string) (*cpv1alpha1.ClusterProfile, error) {
	clusterProfile, err := c.clusterProfileLister.ClusterProfiles(common.KueueNamespace).Get(clusterName)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	clusterManager := clusterProfile.GetLabels()[cpv1alpha1.LabelClusterManagerKey]
	if clusterManager != cpcontroller.ClusterProfileManagerName {
		return nil, fmt.Errorf("cluster manager mismatch: expected %s, got %s from ClusterProfile", cpcontroller.ClusterProfileManagerName, clusterManager)
	}

	extractedClusterName := clusterProfile.GetLabels()[v1.ClusterNameLabelKey]
	if extractedClusterName != clusterName {
		return nil, fmt.Errorf("cluster name mismatch: expected %s, got %s from ClusterProfile", clusterName, extractedClusterName)
	}

	accessProviders := clusterProfile.Status.AccessProviders
	if len(accessProviders) != 1 {
		return nil, fmt.Errorf("expected 1 access provider, got %d", len(accessProviders))
	}

	accessProvider := accessProviders[0]
	if accessProvider.Name != cpcontroller.ClusterProfileManagerName {
		return nil, fmt.Errorf("access provider name mismatch: expected %s, got %s", cpcontroller.ClusterProfileManagerName, accessProvider.Name)
	}

	return clusterProfile, nil
}

// createOrUpdateMultiKueueCluster creates or updates the MultiKueueCluster resource
func (c *multiKueueClusterController) createOrUpdateMultiKueueCluster(ctx context.Context, clusterName string) error {
	logger := klog.FromContext(ctx)

	// Define the desired MultiKueueCluster spec
	desiredSpec := kueuev1beta2.MultiKueueClusterSpec{
		ClusterSource: kueuev1beta2.ClusterSource{
			ClusterProfileRef: &kueuev1beta2.ClusterProfileReference{
				Name: clusterName,
			},
		},
	}

	// Try to get existing MultiKueueCluster
	existingMKC, err := c.kueueClient.KueueV1beta2().MultiKueueClusters().Get(ctx, clusterName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// Create new MultiKueueCluster
			newMKC := &kueuev1beta2.MultiKueueCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: clusterName,
				},
				Spec: desiredSpec,
			}

			_, err = c.kueueClient.KueueV1beta2().MultiKueueClusters().Create(ctx, newMKC, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("failed to create MultiKueueCluster for cluster %s: %v", clusterName, err)
			}

			logger.V(4).Info("MultiKueueCluster created", "cluster", clusterName)
			c.eventRecorder.Eventf("MultiKueueClusterCreated", "Created MultiKueueCluster for cluster %s", clusterName)
			return nil
		}
		return fmt.Errorf("failed to get MultiKueueCluster for cluster %s: %v", clusterName, err)
	}

	// Check if update is needed
	if needsUpdate(existingMKC, desiredSpec) {
		existingMKC.Spec = desiredSpec
		_, err = c.kueueClient.KueueV1beta2().MultiKueueClusters().Update(ctx, existingMKC, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update MultiKueueCluster for cluster %s: %v", clusterName, err)
		}

		logger.V(4).Info("MultiKueueCluster updated", "cluster", clusterName)
		c.eventRecorder.Eventf("MultiKueueClusterUpdated", "Updated MultiKueueCluster for cluster %s", clusterName)
	} else {
		logger.V(4).Info("MultiKueueCluster is up to date", "cluster", clusterName)
	}

	return nil
}

// needsUpdate checks if the MultiKueueCluster needs to be updated
func needsUpdate(existing *kueuev1beta2.MultiKueueCluster, desired kueuev1beta2.MultiKueueClusterSpec) bool {
	// Check if ClusterProfileRef exists in both specs
	if existing.Spec.ClusterSource.ClusterProfileRef == nil && desired.ClusterSource.ClusterProfileRef == nil {
		return false
	}
	if existing.Spec.ClusterSource.ClusterProfileRef == nil || desired.ClusterSource.ClusterProfileRef == nil {
		return true
	}

	// Compare ClusterProfileRef names
	return existing.Spec.ClusterSource.ClusterProfileRef.Name != desired.ClusterSource.ClusterProfileRef.Name
}

// cleanupCluster deletes the MultiKueueCluster resource
func (c *multiKueueClusterController) cleanupCluster(ctx context.Context, clusterName string) error {
	logger := klog.FromContext(ctx)

	err := c.kueueClient.KueueV1beta2().MultiKueueClusters().Delete(ctx, clusterName, metav1.DeleteOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			logger.V(4).Info("MultiKueueCluster already deleted", "cluster", clusterName)
			return nil
		}
		return fmt.Errorf("failed to delete MultiKueueCluster for cluster %s: %v", clusterName, err)
	}

	logger.V(4).Info("MultiKueueCluster deleted", "cluster", clusterName)
	c.eventRecorder.Eventf("MultiKueueClusterDeleted", "Deleted MultiKueueCluster for cluster %s", clusterName)
	return nil
}
