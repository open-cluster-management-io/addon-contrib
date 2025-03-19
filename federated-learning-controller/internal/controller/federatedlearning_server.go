package controller

import (
	"context"
	"fmt"

	routev1 "github.com/openshift/api/route/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	flv1alpha1 "github/open-cluster-management/federated-learning/api/v1alpha1"
	"github/open-cluster-management/federated-learning/internal/controller/manifests"
	"github/open-cluster-management/federated-learning/internal/controller/manifests/applier"
)

// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=create;delete;get;list;watch;update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;delete;create;update
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;delete;create;update
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;delete;update;create
// +kubebuilder:rbac:groups="route.openshift.io",resources=routes,verbs=get;list;watch;create;update;delete

func (r *FederatedLearningReconciler) federatedLearningServer(ctx context.Context, 
	instance *flv1alpha1.FederatedLearning) error {
	// don't delete the storage and cause the job's owner is instance
	if instance.DeletionTimestamp != nil {
		return nil
	}
	if err := r.storage(ctx, instance); err != nil {
		return err
	}

	if len(instance.Spec.Server.Listeners) == 0 {
		return fmt.Errorf("no listeners specified")
	}

	// instance.Spec.Server.Listeners[0].Type != flv1alpha1.Route
	// route is http based -> requires to handle the transport: https://flower.ai/docs/framework/ref-api/flwr.client.start_client.html
	if instance.Spec.Server.Listeners[0].Type != flv1alpha1.LoadBalancer &&
		instance.Spec.Server.Listeners[0].Type != flv1alpha1.NodePort {
		return fmt.Errorf("unsupported listener type: %s", instance.Spec.Server.Listeners[0].Type)
	}

	modelDir, initModel, err := getDirFile(instance.Spec.Server.Storage.ModelPath)
	if err != nil {
		return err
	}

	render, deployer := applier.NewRenderer(manifests.ServerFiles), applier.NewDeployer(r.Client)
	unstructuredObjects, err := render.Render("server", "", func(profile string) (interface{}, error) {
		return manifests.FederatedLearningServerParams{
			Namespace:           instance.Namespace,
			Name:                getSeverName(instance.Name),
			Image:               instance.Spec.Server.Image,
			NumberOfRounds:      instance.Spec.Server.Rounds,
			MinAvailableClients: instance.Spec.Server.MinAvailableClients,
			ModelDir:            modelDir,
			InitModel:           initModel,
			StorageVolumeName:   instance.Spec.Server.Storage.Name,
			ListenerType:        string(instance.Spec.Server.Listeners[0].Type),
			ListenerPort:        instance.Spec.Server.Listeners[0].Port,
		}, nil
	})
	if err != nil {
		return err
	}

	// create discovery client
	dc, err := discovery.NewDiscoveryClientForConfig(r.GetConfig())
	if err != nil {
		return err
	}

	// create restmapper for deployer to find GVR
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))
	if err := SetOwner(unstructuredObjects, instance, mapper, r.Scheme); err != nil {
		return err
	}

	for _, obj := range unstructuredObjects {
		if err := deployer.Deploy(obj); err != nil {
			return err
		}
	}

	if err := r.updateServerAddress(ctx, instance); err != nil {
		return err
	}

	return nil
}

func getSeverName(instanceName string) string {
	return fmt.Sprintf("%s-server", instanceName)
}

// get the address by NodePort, LoadBalancer or Route
func (r *FederatedLearningReconciler) updateServerAddress(ctx context.Context, instance *flv1alpha1.FederatedLearning) error {
	log.Info("update the server address for the clients")
	svc := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: instance.Namespace,
		Name:      getSeverName(instance.Name),
	}, svc); err != nil {
		return err
	}
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		return r.updateLB(ctx, svc, instance)
	}
	// if svc.Spec.Type == corev1.ServiceTypeClusterIP {
	// 	return r.updateRoute(ctx, svc, instance)
	// }
	return fmt.Errorf("failed to update the service address")
}

func (r *FederatedLearningReconciler) updateRoute(ctx context.Context, svc *corev1.Service, instance *flv1alpha1.FederatedLearning) error {
	log.Info("route service found")
	route := &routev1.Route{}

	err := r.Get(ctx, client.ObjectKeyFromObject(instance), route)
	if err != nil {
		return err
	}

	if len(route.Spec.Host) == 0 {
		return fmt.Errorf("Route host is not ready: %s/%s", route.Namespace, route.Name)
	}

	address := route.Spec.Host

	if address != "" {
		newListeners := make([]flv1alpha1.ListenerStatus, 0)
		for _, listener := range instance.Status.Listeners {
			if listener.Type == flv1alpha1.Route {
				continue
			} else {
				newListeners = append(newListeners, listener)
			}
		}
		newListeners = append(newListeners, flv1alpha1.ListenerStatus{
			Name:    fmt.Sprintf("listener(route):%s", route.Name),
			Type:    flv1alpha1.Route,
			Address: address,
			// Port:    svc.Status.LoadBalancer.Ingress[0].Port,
		})

		instance.Status.Listeners = newListeners
		log.Infow("update the server address", "address", address)
		if err := r.Status().Update(ctx, instance); err != nil {
			return err
		}
	} else {
		log.Info("route address is not changed")
	}
	return nil
}

func (r *FederatedLearningReconciler) updateLB(ctx context.Context, svc *corev1.Service, instance *flv1alpha1.FederatedLearning) error {
	log.Info("loadBalancer service found")
	if len(svc.Status.LoadBalancer.Ingress) == 0 {
		log.Info("loadBalancer service address is empty")
		return nil
	}
	address := svc.Status.LoadBalancer.Ingress[0].Hostname + ":" + fmt.Sprintf("%d", svc.Spec.Ports[0].Port)

	if address != "" {
		newListeners := make([]flv1alpha1.ListenerStatus, 0)
		for _, listener := range instance.Status.Listeners {
			if listener.Type == flv1alpha1.LoadBalancer {
				continue
			} else {
				newListeners = append(newListeners, listener)
			}
		}
		newListeners = append(newListeners, flv1alpha1.ListenerStatus{
			Name:    fmt.Sprintf("listener(service):%s", svc.Name),
			Type:    flv1alpha1.LoadBalancer,
			Address: address,
			// Port:    svc.Status.LoadBalancer.Ingress[0].Port,
		})

		instance.Status.Listeners = newListeners
		log.Infow("update the server address", "address", address)
		if err := r.Status().Update(ctx, instance); err != nil {
			return err
		}
	} else if address == "" {
		log.Info("LoadBalancer address is empty")
	} else {
		log.Info("LoadBalancer address is not changed")
	}
	return nil
}

func SetOwner(objects []*unstructured.Unstructured,
	ownerObject client.Object,
	mapper *restmapper.DeferredDiscoveryRESTMapper, scheme *runtime.Scheme,
) error {
	// manipulate the object
	for _, obj := range objects {
		// don't add the svc for now
		if obj.GroupVersionKind().GroupKind().Kind == "Service" {
			continue
		}
		mapping, err := mapper.RESTMapping(obj.GroupVersionKind().GroupKind(), obj.GroupVersionKind().Version)
		if err != nil {
			return err
		}

		if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
			// for namespaced resource, set ownerreference of controller
			if err := controllerutil.SetControllerReference(ownerObject, obj, scheme); err != nil {
				return err
			}
		}

		// // set owner labels
		// labels := obj.GetLabels()
		// if labels == nil {
		// 	labels = make(map[string]string)
		// }
		// labels[constants.GlobalHubOwnerLabelKey] = constants.GHOperatorOwnerLabelVal
		// obj.SetLabels(labels)
	}
	return nil
}

func (r *FederatedLearningReconciler) storage(ctx context.Context, instance *flv1alpha1.FederatedLearning) error {
	namespace := instance.Namespace
	name := instance.Spec.Server.Storage.Name
	size := instance.Spec.Server.Storage.Size
	storageType := instance.Spec.Server.Storage.Type
	if storageType != flv1alpha1.PersistentVolumeClaim {
		return fmt.Errorf("unsupported storage type: %s", storageType)
	}

	pvc := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, pvc)
	if err != nil {
		if errors.IsNotFound(err) {
			// PVC does not exist, create it
			newPVC := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse(size),
						},
					},
				},
			}
			if err := r.Create(ctx, newPVC); err != nil {
				return err
			}
			log.Infow("created PVC", "name", name, "namespace", namespace)
			return nil
		}
		return err
	}

	// PVC exists
	log.Infof("storage PVC already exists: %s", name)
	return nil
}
