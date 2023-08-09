package controllers

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"open-cluster-management.io/addon-framework/pkg/basecontroller/factory"

	"open-cluster-management-io/addon-contrib/device-addon/pkg/addon/patcher"
	"open-cluster-management-io/addon-contrib/device-addon/pkg/apis/v1alpha1"
	deviceclient "open-cluster-management-io/addon-contrib/device-addon/pkg/client/clientset/versioned"
	deviceinformerv1alpha1 "open-cluster-management-io/addon-contrib/device-addon/pkg/client/informers/externalversions/apis/v1alpha1"
	devicelisterv1alpha1 "open-cluster-management-io/addon-contrib/device-addon/pkg/client/listers/apis/v1alpha1"
	"open-cluster-management-io/addon-contrib/device-addon/pkg/device/equipment"
)

const driverFinalizer = "edge.open-cluster-management.io/driver-cleanup"

type driversController struct {
	client      deviceclient.Interface
	lister      devicelisterv1alpha1.DriverLister
	equipment   *equipment.Equipment
	clusterName string
	patcher     patcher.Patcher[*v1alpha1.Driver, v1alpha1.DriverSpec, v1alpha1.DriverStatus]
}

func NewDriversController(
	clusterName string,
	client deviceclient.Interface,
	driverInformer deviceinformerv1alpha1.DriverInformer,
	equipment *equipment.Equipment,
) factory.Controller {
	c := &driversController{
		client:      client,
		lister:      driverInformer.Lister(),
		equipment:   equipment,
		clusterName: clusterName,
		patcher: patcher.NewPatcher[*v1alpha1.Driver, v1alpha1.DriverSpec, v1alpha1.DriverStatus](
			client.EdgeV1alpha1().Drivers(clusterName)),
	}

	return factory.New().
		WithInformersQueueKeysFunc(func(obj runtime.Object) []string {
			key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			return []string{key}
		}, driverInformer.Informer()).
		WithSync(c.sync).
		ToController("driver-controller")
}

func (c *driversController) sync(ctx context.Context, syncCtx factory.SyncContext, key string) error {
	_, driverName, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		// ignore addon whose key is invalid
		klog.Warningf("driver key %s is invalid, %v", key, err)
		return nil
	}

	klog.Infof("sync driver %s/%s", c.clusterName, driverName)

	driver, err := c.lister.Drivers(c.clusterName).Get(driverName)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	if !driver.DeletionTimestamp.IsZero() {
		if err := c.equipment.UnInstallDriver(driver.Spec.DriverConfig); err != nil {
			return err
		}

		return c.patcher.RemoveFinalizer(ctx, driver, driverFinalizer)
	}

	updated, err := c.patcher.AddFinalizer(ctx, driver, driverFinalizer)
	if err != nil || updated {
		return err
	}

	installedCondition := metav1.Condition{
		Type:    "DriverInstalled",
		Status:  metav1.ConditionTrue,
		Reason:  "DriverInstalled",
		Message: "Driver is installed",
	}

	if err := c.equipment.InstallDriver(driver.Spec.DriverConfig); err != nil {
		installedCondition.Status = metav1.ConditionFalse
		installedCondition.Reason = "DriverNotInstalled"
		installedCondition.Message = fmt.Sprintf("Driver is failed to install, %v", err)
	}

	newDriver := driver.DeepCopy()
	meta.SetStatusCondition(&newDriver.Status.Conditions, installedCondition)

	_, updatedErr := c.patcher.PatchStatus(ctx, newDriver, newDriver.Status, driver.Status)
	return updatedErr
}
