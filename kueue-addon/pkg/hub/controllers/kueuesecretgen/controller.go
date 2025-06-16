package kueuesecretgen

import (
	"context"
	"fmt"

	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"open-cluster-management.io/addon-contrib/kueue-addon/manifests"
	"open-cluster-management.io/addon-contrib/kueue-addon/pkg/hub/controllers/common"
	clusterinformerv1 "open-cluster-management.io/api/client/cluster/informers/externalversions/cluster/v1"
	clusterlisterv1 "open-cluster-management.io/api/client/cluster/listers/cluster/v1"
	permissionclientset "open-cluster-management.io/cluster-permission/client/clientset/versioned"
	permissioninformer "open-cluster-management.io/cluster-permission/client/informers/externalversions/api/v1alpha1"
	permissionlisterv1alpha1 "open-cluster-management.io/cluster-permission/client/listers/api/v1alpha1"
	msaclientset "open-cluster-management.io/managed-serviceaccount/pkg/generated/clientset/versioned"
	msainformer "open-cluster-management.io/managed-serviceaccount/pkg/generated/informers/externalversions/authentication/v1beta1"
	msalisterv1beta1 "open-cluster-management.io/managed-serviceaccount/pkg/generated/listers/authentication/v1beta1"

	"open-cluster-management.io/ocm/pkg/common/queue"
)

var (
	clusterPermissionFile = "cluster-permission/cluster-permission.yaml"
)

// kueueSecretGenController reconciles instances of ClusterPermission and ManagedServiceAccount on the hub.
type kueueSecretGenController struct {
	permissionClient permissionclientset.Interface
	msaClient        msaclientset.Interface
	clusterLister    clusterlisterv1.ManagedClusterLister
	permissionLister permissionlisterv1alpha1.ClusterPermissionLister
	msaLister        msalisterv1beta1.ManagedServiceAccountLister
	eventRecorder    events.Recorder
}

// NewkueueSecretGenController creates a new controller that create ClusterPermission and ManagedServiceAccount
// for spoke clusters
func NewkueueSecretGenController(
	permissionClient permissionclientset.Interface,
	msaClient msaclientset.Interface,
	clusterInformer clusterinformerv1.ManagedClusterInformer,
	permissionInformers permissioninformer.ClusterPermissionInformer,
	msaInformers msainformer.ManagedServiceAccountInformer,
	recorder events.Recorder) factory.Controller {
	c := &kueueSecretGenController{
		permissionClient: permissionClient,
		msaClient:        msaClient,
		clusterLister:    clusterInformer.Lister(),
		permissionLister: permissionInformers.Lister(),
		msaLister:        msaInformers.Lister(),
		eventRecorder:    recorder.WithComponentSuffix("kueue-secret-gen-controller"),
	}

	return factory.New().
		WithInformersQueueKeysFunc(queue.QueueKeyByMetaName,
			clusterInformer.Informer()).
		WithInformersQueueKeysFunc(queue.QueueKeyByMetaNamespace,
			permissionInformers.Informer(),
			msaInformers.Informer()).
		WithSync(c.sync).
		ToController("kueueSecretGenController", recorder)
}

func (c *kueueSecretGenController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	managedClusterName := syncCtx.QueueKey()
	logger := klog.FromContext(ctx)
	logger.Info("Reconciling Cluster", "key", managedClusterName)

	managedCluster, err := c.clusterLister.Get(managedClusterName)
	if errors.IsNotFound(err) {
		logger.Info("Managed cluster not found, skipping reconciliation", "cluster", managedClusterName)
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get managed cluster %s: %v", managedClusterName, err)
	}

	// if the managed cluster is deleting, delete the clusterpermission, managedserviceaccount as well.
	if !managedCluster.DeletionTimestamp.IsZero() {
		logger.Info("Managed cluster is being deleted, cleaning up resources", "cluster", managedClusterName)

		// Delete ClusterPermission
		if err := c.permissionClient.ApiV1alpha1().ClusterPermissions(managedClusterName).Delete(ctx, common.MultiKueueResourceName, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete ClusterPermission %s in cluster %s: %v", common.MultiKueueResourceName, managedClusterName, err)
		}

		// Delete ManagedServiceAccount
		if err := c.msaClient.AuthenticationV1beta1().ManagedServiceAccounts(managedClusterName).Delete(ctx, common.MultiKueueResourceName, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete ManagedServiceAccount %s in cluster %s: %v", common.MultiKueueResourceName, managedClusterName, err)
		}

		return nil
	}

	// Apply ClusterPermission from yaml
	if err := applyClusterPermission(
		ctx,
		c.permissionClient,
		func(name string) ([]byte, error) {
			return manifests.ClusterPermissionManifestFiles.ReadFile(name)
		},
		clusterPermissionFile,
		managedClusterName,
	); err != nil {
		return fmt.Errorf("failed to apply cluster permission: %v", err)
	}
	logger.Info("ClusterPermission applied", "namespace", managedClusterName)

	// Apply ManagedServiceAccount
	if err := applyManagedServiceAccount(
		ctx,
		c.msaClient,
		managedClusterName,
	); err != nil {
		return fmt.Errorf("failed to apply managed service account: %v", err)
	}
	logger.Info("ManagedServiceAccount applied", "name", common.MultiKueueResourceName, "namespace", managedClusterName)

	return nil
}
