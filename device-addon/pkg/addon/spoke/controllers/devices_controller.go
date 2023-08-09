package controllers

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"open-cluster-management.io/addon-framework/pkg/basecontroller/factory"

	"open-cluster-management-io/addon-contrib/device-addon/pkg/addon/patcher"
	"open-cluster-management-io/addon-contrib/device-addon/pkg/apis/v1alpha1"
	deviceclient "open-cluster-management-io/addon-contrib/device-addon/pkg/client/clientset/versioned"
	deviceinformerv1alpha1 "open-cluster-management-io/addon-contrib/device-addon/pkg/client/informers/externalversions/apis/v1alpha1"
	devicelisterv1alpha1 "open-cluster-management-io/addon-contrib/device-addon/pkg/client/listers/apis/v1alpha1"
	"open-cluster-management-io/addon-contrib/device-addon/pkg/device/equipment"
)

const deviceFinalizer = "edge.open-cluster-management.io/device-cleanup"

type devicesController struct {
	client      deviceclient.Interface
	lister      devicelisterv1alpha1.DeviceLister
	equipment   *equipment.Equipment
	clusterName string
	patcher     patcher.Patcher[*v1alpha1.Device, v1alpha1.DeviceSpec, v1alpha1.DeviceStatus]
}

func NewDevicesController(
	clusterName string,
	client deviceclient.Interface,
	deviceInformer deviceinformerv1alpha1.DeviceInformer,
	equipment *equipment.Equipment,
) factory.Controller {
	c := &devicesController{
		client:      client,
		lister:      deviceInformer.Lister(),
		equipment:   equipment,
		clusterName: clusterName,
		patcher: patcher.NewPatcher[*v1alpha1.Device, v1alpha1.DeviceSpec, v1alpha1.DeviceStatus](
			client.EdgeV1alpha1().Devices(clusterName)),
	}

	return factory.New().
		WithInformersQueueKeysFunc(func(obj runtime.Object) []string {
			key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			return []string{key}
		}, deviceInformer.Informer()).
		WithSync(c.sync).
		ToController("device-controller")
}

func (c *devicesController) sync(ctx context.Context, syncCtx factory.SyncContext, key string) error {
	_, deviceName, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		// ignore addon whose key is invalid
		klog.Warningf("device key %s is invalid, %v", key, err)
		return nil
	}

	klog.Infof("sync device %s/%s", c.clusterName, deviceName)

	device, err := c.lister.Devices(c.clusterName).Get(deviceName)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	//

	driver := c.equipment.GetDriver(device.Spec.DriverType)
	if driver == nil {
		// requeue
		syncCtx.Queue().AddAfter(key, 5*time.Second)
		return nil
	}

	if !device.DeletionTimestamp.IsZero() {
		if err := driver.RemoveDevice(device.Spec.Name); err != nil {
			return err
		}

		return c.patcher.RemoveFinalizer(ctx, device, deviceFinalizer)
	}

	updated, err := c.patcher.AddFinalizer(ctx, device, deviceFinalizer)
	if err != nil || updated {
		return err
	}

	addedCondition := metav1.Condition{
		Type:    "DeviceAdded",
		Status:  metav1.ConditionTrue,
		Reason:  "DeviceAdded",
		Message: "Device is added",
	}

	if err := driver.AddDevice(device.Spec.DeviceConfig); err != nil {
		addedCondition.Status = metav1.ConditionFalse
		addedCondition.Reason = "DeviceNotAdded"
		addedCondition.Message = fmt.Sprintf("Device is failed to add, %v", err)
	}

	newDevice := device.DeepCopy()
	meta.SetStatusCondition(&newDevice.Status.Conditions, addedCondition)

	_, updatedErr := c.patcher.PatchStatus(ctx, newDevice, newDevice.Status, device.Status)
	return updatedErr
}
