// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"
	v1alpha1 "open-cluster-management-io/addon-contrib/device-addon/pkg/apis/v1alpha1"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeDrivers implements DriverInterface
type FakeDrivers struct {
	Fake *FakeEdgeV1alpha1
	ns   string
}

var driversResource = v1alpha1.SchemeGroupVersion.WithResource("drivers")

var driversKind = v1alpha1.SchemeGroupVersion.WithKind("Driver")

// Get takes name of the driver, and returns the corresponding driver object, and an error if there is any.
func (c *FakeDrivers) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.Driver, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(driversResource, c.ns, name), &v1alpha1.Driver{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Driver), err
}

// List takes label and field selectors, and returns the list of Drivers that match those selectors.
func (c *FakeDrivers) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.DriverList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(driversResource, driversKind, c.ns, opts), &v1alpha1.DriverList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.DriverList{ListMeta: obj.(*v1alpha1.DriverList).ListMeta}
	for _, item := range obj.(*v1alpha1.DriverList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested drivers.
func (c *FakeDrivers) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(driversResource, c.ns, opts))

}

// Create takes the representation of a driver and creates it.  Returns the server's representation of the driver, and an error, if there is any.
func (c *FakeDrivers) Create(ctx context.Context, driver *v1alpha1.Driver, opts v1.CreateOptions) (result *v1alpha1.Driver, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(driversResource, c.ns, driver), &v1alpha1.Driver{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Driver), err
}

// Update takes the representation of a driver and updates it. Returns the server's representation of the driver, and an error, if there is any.
func (c *FakeDrivers) Update(ctx context.Context, driver *v1alpha1.Driver, opts v1.UpdateOptions) (result *v1alpha1.Driver, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(driversResource, c.ns, driver), &v1alpha1.Driver{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Driver), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeDrivers) UpdateStatus(ctx context.Context, driver *v1alpha1.Driver, opts v1.UpdateOptions) (*v1alpha1.Driver, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(driversResource, "status", c.ns, driver), &v1alpha1.Driver{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Driver), err
}

// Delete takes name of the driver and deletes it. Returns an error if one occurs.
func (c *FakeDrivers) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(driversResource, c.ns, name, opts), &v1alpha1.Driver{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeDrivers) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(driversResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha1.DriverList{})
	return err
}

// Patch applies the patch and returns the patched driver.
func (c *FakeDrivers) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.Driver, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(driversResource, c.ns, name, pt, data, subresources...), &v1alpha1.Driver{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Driver), err
}
