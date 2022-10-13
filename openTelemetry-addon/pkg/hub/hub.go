package hub

import (
	"context"
	"embed"
	"github.com/pkg/errors"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	"open-cluster-management.io/addon-framework/pkg/assets"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	genericScheme = runtime.NewScheme()
	genericCodecs = serializer.NewCodecFactory(genericScheme)
	genericCodec  = genericCodecs.UniversalDeserializer()
)

func init() {
	utilruntime.Must(scheme.AddToScheme(genericScheme))
	utilruntime.Must(v1.AddToScheme(genericScheme))
	utilruntime.Must(addonv1alpha1.AddToScheme(genericScheme))
}

//go:embed manifests
var fs embed.FS

var manifestFiles = []string{
	"manifests/service_account.yaml",
	"manifests/jaeger_deployment.yaml",
	"manifests/jaeger_service.yaml",
	"manifests/collector_deployment.yaml",
	"manifests/collector_service.yaml",
	"manifests/collector_config.yaml",
	"manifests/jaeger_external.yaml",
}

func Applymanifests(rclient client.Client) error {
	for _, file := range manifestFiles {
		template, err := fs.ReadFile(file)
		if err != nil {
			return err
		}
		raw := assets.MustCreateAssetFromTemplate(file, template, nil).Data
		obj, gvk, err := genericCodec.Decode(raw, nil, nil)
		if err != nil {
			klog.ErrorS(err, "Error decoding manifest file", "filename", file)
			return err
		}
		resource := obj.(client.Object)
		resource.SetOwnerReferences([]metav1.OwnerReference{
			ownerrefernce(rclient),
		})
		err = deploy(rclient, resource, *gvk)
		if err != nil {
			return err
		}
	}
	return nil
}

func deploy(rclient client.Client, resource client.Object, gvk schema.GroupVersionKind) error {

	current := &unstructured.Unstructured{}
	current.SetGroupVersionKind(gvk)
	if err := rclient.Get(
		context.TODO(),
		types.NamespacedName{
			Namespace: resource.GetNamespace(),
			Name:      resource.GetName(),
		}, current); err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrapf(err,
				"failed to get obj kind: %s, namespace: %s, name %s",
				gvk.Kind,
				resource.GetNamespace(),
				resource.GetName(),
			)
		}
		// if not found, then create
		if err := rclient.Create(context.TODO(), resource); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				return errors.Wrapf(err,
					"failed to create obj kind: %s, namespace: %s, name %s",
					gvk.Kind,
					resource.GetNamespace(),
					resource.GetName(),
				)
			}
		}
	}
	return nil
}

func ownerrefernce(rclient client.Client) metav1.OwnerReference {
	current := &unstructured.Unstructured{}
	current.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   addonv1alpha1.GroupName,
		Version: addonv1alpha1.GroupVersion.Version,
		Kind:    "ClusterManagementAddOn",
	})
	err := rclient.Get(
		context.TODO(),
		types.NamespacedName{
			Namespace: "open-cluster-management-addon",
			Name:      "otel-collector",
		}, current)
	if err != nil {
		klog.ErrorS(err, "Error getting owner refernece object")

	}
	return metav1.OwnerReference{
		APIVersion: current.GetAPIVersion(),
		Kind:       "ClusterManagementAddOn",
		Name:       current.GetName(),
		UID:        current.GetUID(),
	}
}
