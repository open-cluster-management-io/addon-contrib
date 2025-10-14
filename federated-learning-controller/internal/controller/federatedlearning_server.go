package controller

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"strings"

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
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github/open-cluster-management/federated-learning/api/v1alpha1"
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
	instance *flv1alpha1.FederatedLearning,
) error {
	// don't delete the storage and cause the job's owner is instance
	if instance.DeletionTimestamp != nil {
		return nil
	}

	if len(instance.Spec.Server.Listeners) == 0 {
		return fmt.Errorf("no listeners specified")
	}

	var err error
	defer func() {
		if err != nil {
			instance.Status.Message = fmt.Sprintf("failed to initialize the server resources: %s", err.Error())
			if updateErr := r.Status().Update(ctx, instance); updateErr != nil {
				log.Errorw("failed to update the instance status", "error", updateErr)
			}
			return
		}
	}()

	if err = r.storage(ctx, instance); err != nil {
		return err
	}

	// instance.Spec.Server.Listeners[0].Type != flv1alpha1.Route
	// route is http based -> requires to handle the transport: https://flower.ai/docs/framework/ref-api/flwr.client.start_client.html
	if instance.Spec.Server.Listeners[0].Type != flv1alpha1.LoadBalancer &&
		instance.Spec.Server.Listeners[0].Type != flv1alpha1.NodePort {
		err = fmt.Errorf("unsupported listener type: %s", instance.Spec.Server.Listeners[0].Type)
		return err
	}

	createService := false

	// get the servcie instance if not exist create it
	service := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: instance.Namespace,
		Name:      getSeverName(instance.Name),
	}, service); err != nil {
		if errors.IsNotFound(err) {
			createService = true
		} else {
			err = fmt.Errorf("failed to get service: %w", err)
			return err
		}
	} else {
		// if service already exists, check if the type is correct
		if service.Spec.Type != corev1.ServiceType(instance.Spec.Server.Listeners[0].Type) {
			log.Infof("service type is %s, but expected %s", service.Spec.Type, instance.Spec.Server.Listeners[0].Type)
			createService = true
		}
	}

	// For LoadBalancer, we need to ensure the service is created and has an external IP before creating the job
	if instance.Spec.Server.Listeners[0].Type == flv1alpha1.LoadBalancer {
		if createService {
			// Create service first
			if err = r.createServiceForLoadBalancer(ctx, instance); err != nil {
				return err
			}
		}

		// Wait for LoadBalancer external IP to be assigned
		if err = r.waitForLoadBalancerIP(ctx, instance); err != nil {
			return err
		}
	}

	modelDir, initModel, err := getDirFile(instance.Spec.Server.Storage.ModelPath)
	if err != nil {
		return err
	}

	obsSidecarImage := ""
	if instance.ObjectMeta.Annotations != nil {
		obsSidecarImage = instance.ObjectMeta.Annotations[v1alpha1.AnnotationSidecarImage]
	}

	var serverParams any
	var serverFS embed.FS

	switch instance.Spec.Framework {
	case flv1alpha1.Flower:
		serverFS = manifests.FlowerServerFiles
		serverParams = &manifests.FlowerServerParams{
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
			CreateService:       createService,
			ObsSidecarImage:     obsSidecarImage,
		}
	case flv1alpha1.OpenFL:
		serverFS = manifests.OpenFLServerFiles
		clusters, err := r.getDecidedClusters(ctx, instance)
		if err != nil {
			return err
		}
		sort.Strings(clusters)
		log.Infof("clusters: %+v", clusters)
		// determine endpoint info (IP and port) prior to rendering, especially for NodePort
		listenerIP, listenerPort, err := r.determineEndpointInfo(ctx, instance)
		if err != nil {
			return err
		}
		serverParams = &manifests.OpenFLServerParams{
			Namespace:         instance.Namespace,
			Name:              getSeverName(instance.Name),
			Image:             instance.Spec.Server.Image,
			NumberOfRounds:    instance.Spec.Server.Rounds,
			StorageVolumeName: instance.Spec.Server.Storage.Name,
			ListenerType:      string(instance.Spec.Server.Listeners[0].Type),
			ListenerIP:        listenerIP,
			ListenerPort:      listenerPort,
			CreateService:     createService,
			ModelDir:          modelDir,
			ObsSidecarImage:   obsSidecarImage,
			Collaborators:     strings.Join(clusters, ","),
		}
		log.Infof("server params: %+v", serverParams)
	default:
		return fmt.Errorf("unsupported framework: %s", instance.Spec.Framework)
	}

	render, deployer := applier.NewRenderer(serverFS), applier.NewDeployer(r.Client)
	unstructuredObjects, err := render.Render("", "", func(profile string) (interface{}, error) {
		return serverParams, nil
	})

	// create discovery client
	dc, err := discovery.NewDiscoveryClientForConfig(r.GetConfig())
	if err != nil {
		return err
	}

	// create restmapper for deployer to find GVR
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))
	if err = SetOwner(unstructuredObjects, instance, mapper, r.Scheme); err != nil {
		return err
	}

	for _, obj := range unstructuredObjects {
		log.Infof("deploying %s/%s", obj.GetNamespace(), obj.GetName())
		if err := deployer.Deploy(obj); err != nil {
			return err
		}
	}

	if err = r.updateServerAddress(ctx, instance); err != nil {
		return err
	}

	return nil
}

// createServiceForLoadBalancer creates the LoadBalancer service first
func (r *FederatedLearningReconciler) createServiceForLoadBalancer(ctx context.Context, instance *flv1alpha1.FederatedLearning) error {
	log.Info("creating LoadBalancer service first")

	modelDir, _, err := getDirFile(instance.Spec.Server.Storage.ModelPath)
	if err != nil {
		return err
	}

	obsSidecarImage := ""
	if instance.ObjectMeta.Annotations != nil {
		obsSidecarImage = instance.ObjectMeta.Annotations[v1alpha1.AnnotationSidecarImage]
	}

	serverParams := &manifests.OpenFLServerParams{
		Namespace:         instance.Namespace,
		Name:              getSeverName(instance.Name),
		Image:             instance.Spec.Server.Image,
		NumberOfRounds:    instance.Spec.Server.Rounds,
		StorageVolumeName: instance.Spec.Server.Storage.Name,
		ListenerType:      string(instance.Spec.Server.Listeners[0].Type),
		ListenerIP:        "", // Will be updated after LoadBalancer IP is assigned
		ListenerPort:      instance.Spec.Server.Listeners[0].Port,
		CreateService:     true,
		ModelDir:          modelDir,
		ObsSidecarImage:   obsSidecarImage,
		Collaborators:     "", // Will be updated later
	}

	// Only render and deploy the service, not the job
	render := applier.NewRenderer(manifests.OpenFLServerFiles)
	unstructuredObjects, err := render.Render("", "", func(profile string) (interface{}, error) {
		return serverParams, nil
	})
	if err != nil {
		return err
	}

	// Filter to only deploy the service
	for _, obj := range unstructuredObjects {
		if obj.GetKind() == "Service" {
			log.Infof("deploying service %s/%s", obj.GetNamespace(), obj.GetName())
			deployer := applier.NewDeployer(r.Client)
			if err := deployer.Deploy(obj); err != nil {
				return err
			}
		}
	}

	return nil
}

// waitForLoadBalancerIP waits for the LoadBalancer service to get an external IP
func (r *FederatedLearningReconciler) waitForLoadBalancerIP(ctx context.Context, instance *flv1alpha1.FederatedLearning) error {
	log.Info("checking LoadBalancer external IP readiness")

	serviceName := getSeverName(instance.Name)

	service := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: instance.Namespace,
		Name:      serviceName,
	}, service); err != nil {
		return fmt.Errorf("failed to get service: %w", err)
	}

	if len(service.Status.LoadBalancer.Ingress) > 0 {
		ingress := service.Status.LoadBalancer.Ingress[0]
		if ingress.IP != "" || ingress.Hostname != "" {
			log.Infof("LoadBalancer external address assigned: %v", service.Status.LoadBalancer.Ingress)
			return nil
		}
	}

	return fmt.Errorf("LoadBalancer external address not ready")
}

func (r *FederatedLearningReconciler) getDecidedClusters(ctx context.Context, instance *flv1alpha1.FederatedLearning) ([]string, error) {
	clusterNames := make([]string, 0)
	placement := &clusterv1beta1.Placement{
		ObjectMeta: metav1.ObjectMeta{
			Name: instance.Name, Namespace: instance.Namespace,
		},
	}
	if err := r.Get(ctx, client.ObjectKeyFromObject(placement), placement); err != nil {
		return nil, err
	}

	for _, decisionGroup := range placement.Status.DecisionGroups {
		for _, decisionName := range decisionGroup.Decisions {
			decision := clusterv1beta1.PlacementDecision{}
			if err := r.Get(ctx, types.NamespacedName{
				Namespace: instance.Namespace, Name: decisionName,
			}, &decision); err != nil {
				return nil, err
			}

			for _, clusterDecision := range decision.Status.Decisions {
				cluster := &clusterv1.ManagedCluster{}
				if err := r.Get(ctx, types.NamespacedName{Name: clusterDecision.ClusterName}, cluster); err != nil {
					return nil, err
				}
				clusterNames = append(clusterNames, cluster.Name)
			}
		}
	}
	log.Infof("decided clusters: %v", clusterNames)
	return clusterNames, nil
}

func getSeverName(instanceName string) string {
	return fmt.Sprintf("%s-server", instanceName)
}

// pruneServerResources cleans up server-side Kubernetes resources when the instance is deleted.
func (r *FederatedLearningReconciler) pruneServerResources(ctx context.Context, instance *flv1alpha1.FederatedLearning) error {
	svc := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: instance.Namespace,
		Name:      getSeverName(instance.Name),
	}, svc); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get service during deletion: %w", err)
	}

	if err := r.Delete(ctx, svc); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete service during deletion: %w", err)
	}

	log.Infof("deleted service %s/%s while cleaning up FederatedLearning instance", svc.Namespace, svc.Name)
	return nil
}

// determineEndpointInfo determines endpoint information before service creation
func (r *FederatedLearningReconciler) determineEndpointInfo(ctx context.Context, instance *flv1alpha1.FederatedLearning) (string, int, error) {
	listenerType := instance.Spec.Server.Listeners[0].Type
	port := instance.Spec.Server.Listeners[0].Port

	switch listenerType {
	case flv1alpha1.LoadBalancer:
		// For LoadBalancer, try to get the external IP from existing service
		serviceName := getSeverName(instance.Name)
		service := &corev1.Service{}
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: instance.Namespace,
			Name:      serviceName,
		}, service); err == nil {
			// Service exists, check if it has external IP
			if len(service.Status.LoadBalancer.Ingress) > 0 {
				ingress := service.Status.LoadBalancer.Ingress[0]
				if ingress.IP != "" {
					log.Infof("Found LoadBalancer external IP: %s", ingress.IP)
					return ingress.IP, port, nil
				} else if ingress.Hostname != "" {
					log.Infof("Found LoadBalancer external hostname: %s", ingress.Hostname)
					return ingress.Hostname, port, nil
				}
			}
		}
		// If service doesn't exist or doesn't have external IP yet, return empty
		// This will be handled by the calling function
		return "", port, nil

	case flv1alpha1.NodePort:
		// For NodePort, we can determine the node IP beforehand
		nodeIp := instance.Spec.Server.Listeners[0].IP
		if nodeIp == "" {
			// Get the first node's internal IP
			nodeList := &corev1.NodeList{}
			if err := r.List(ctx, nodeList); err != nil {
				return "", 0, err
			}

			for _, node := range nodeList.Items {
				for _, addr := range node.Status.Addresses {
					if addr.Address != "" && addr.Type == corev1.NodeInternalIP {
						nodeIp = addr.Address
						break
					}
				}
				if nodeIp != "" {
					break
				}
			}
		}

		if nodeIp == "" {
			return "", 0, fmt.Errorf("no node internal IP found")
		}

		// For NodePort, we can use the same port as specified in the listener
		// The actual nodePort will be assigned by Kubernetes, but we can use the target port
		return nodeIp, port, nil

	default:
		return "", 0, fmt.Errorf("unsupported listener type: %s", listenerType)
	}
}

// get the address by NodePort, LoadBalancer or Route
func (r *FederatedLearningReconciler) updateServerAddress(ctx context.Context, instance *flv1alpha1.FederatedLearning) error {
	log.Info("update the server address for the clients")
	svc := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: instance.Namespace,
		Name:      getSeverName(instance.Name),
	}, svc); err != nil {
		log.Infof("failed to get service: %v", err)
		return err
	}
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		return r.updateLB(ctx, svc, instance)
	}
	if svc.Spec.Type == corev1.ServiceTypeNodePort {
		return r.updateNP(ctx, svc, instance)
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
		return fmt.Errorf("loadBalancer service address is empty for %s/%s", svc.Namespace, svc.Name)
	}

	var address string
	ingress := svc.Status.LoadBalancer.Ingress[0]
	port := fmt.Sprintf("%d", svc.Spec.Ports[0].Port)

	if ingress.Hostname != "" {
		address = ingress.Hostname + ":" + port
	} else if ingress.IP != "" {
		address = ingress.IP + ":" + port
	} else {
		return fmt.Errorf("no available ingress address in service")
	}

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

func (r *FederatedLearningReconciler) updateNP(ctx context.Context, svc *corev1.Service, instance *flv1alpha1.FederatedLearning) error {
	log.Info("nodePort service found")
	if len(svc.Spec.Ports) == 0 {
		log.Info("nodePort service ports is empty")
		return nil
	}

	nodePort := svc.Spec.Ports[0].NodePort
	if nodePort == 0 {
		log.Info("nodePort is 0")
		return nil
	}

	nodeIp := instance.Spec.Server.Listeners[0].IP
	if nodeIp == "" {
		log.Info("node IP is not specified, will use the first node's internal IP")

		nodeList := &corev1.NodeList{}
		if err := r.List(ctx, nodeList); err != nil {
			return err
		}

		for _, node := range nodeList.Items {
			for _, addr := range node.Status.Addresses {
				if addr.Address != "" && addr.Type == corev1.NodeInternalIP {
					nodeIp = addr.Address
					break
				}
			}
		}
	}

	if nodeIp == "" {
		return fmt.Errorf("no node internal IP found")
	}

	address := fmt.Sprintf("%s:%d", nodeIp, nodePort)
	log.Infof("found address: %s", address)

	newListeners := make([]flv1alpha1.ListenerStatus, 0)
	for _, listener := range instance.Status.Listeners {
		if listener.Type == flv1alpha1.NodePort {
			continue
		} else {
			newListeners = append(newListeners, listener)
		}
	}

	newListeners = append(newListeners, flv1alpha1.ListenerStatus{
		Name:    fmt.Sprintf("listener(service):%s", svc.Name),
		Type:    flv1alpha1.NodePort,
		Address: address,
		Port:    int(nodePort),
	})

	instance.Status.Listeners = newListeners
	log.Infow("update the server address", "address", address)
	if err := r.Status().Update(ctx, instance); err != nil {
		return err
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
	storageType := instance.Spec.Server.Storage.Type

	switch storageType {
	case flv1alpha1.PVCStorage:
		return r.ensureStandardPVC(ctx, instance)
	case flv1alpha1.S3Bucket:
		return r.ensureS3PVC(ctx, instance)
	default:
		return fmt.Errorf("unsupported storage type: %s", storageType)
	}
}

func (r *FederatedLearningReconciler) ensureStandardPVC(ctx context.Context, instance *flv1alpha1.FederatedLearning) error {
	namespace := instance.Namespace
	name := instance.Spec.Server.Storage.Name

	if instance.Spec.Server.Storage.Size == "" {
		return fmt.Errorf("size must be specified for storage type %s", instance.Spec.Server.Storage.Type)
	}

	pvc := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, pvc)
	if err != nil {
		if errors.IsNotFound(err) {
			quantity, parseErr := resource.ParseQuantity(instance.Spec.Server.Storage.Size)
			if parseErr != nil {
				return fmt.Errorf("failed to parse storage size %q: %w", instance.Spec.Server.Storage.Size, parseErr)
			}
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
							corev1.ResourceStorage: quantity,
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

	log.Infof("storage PVC already exists: %s", name)
	return nil
}

func (r *FederatedLearningReconciler) ensureS3PVC(ctx context.Context, instance *flv1alpha1.FederatedLearning) error {
	storageSpec := instance.Spec.Server.Storage
	namespace := instance.Namespace
	claimName := storageSpec.Name

	if storageSpec.Size == "" {
		return fmt.Errorf("size must be specified for storage type %s", storageSpec.Type)
	}
	if storageSpec.S3 == nil {
		return fmt.Errorf("s3 configuration must be provided when storage type is %s", storageSpec.Type)
	}
	if storageSpec.S3.BucketName == "" {
		return fmt.Errorf("bucketName is required for s3 storage")
	}

	requestQuantity, err := resource.ParseQuantity(storageSpec.Size)
	if err != nil {
		return fmt.Errorf("failed to parse storage size %q: %w", storageSpec.Size, err)
	}
	pvName := fmt.Sprintf("%s-pv", claimName)

	mountOptions := make([]string, 0)
	if storageSpec.S3.Region != "" {
		mountOptions = append(mountOptions, fmt.Sprintf("region %s", storageSpec.S3.Region))
	}
	if storageSpec.S3.Prefix != "" {
		mountOptions = append(mountOptions, fmt.Sprintf("prefix %s", storageSpec.S3.Prefix))
	}
	desiredPV := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvName,
			Namespace: namespace,
		},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			},
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: requestQuantity,
			},
			ClaimRef: &corev1.ObjectReference{
				Namespace: namespace,
				Name:      claimName,
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:       v1alpha1.S3Driver,
					VolumeHandle: v1alpha1.S3VolumeHandle,
					VolumeAttributes: map[string]string{
						"bucketName": storageSpec.S3.BucketName,
					},
				},
			},
			MountOptions:     mountOptions,
			StorageClassName: "",
		},
	}
	existingPV := &corev1.PersistentVolume{}
	if err := r.Get(ctx, client.ObjectKey{Name: pvName}, existingPV); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		if err := r.Create(ctx, desiredPV); err != nil {
			return fmt.Errorf("failed to create s3 persistent volume %q: %w", pvName, err)
		}
		log.Infow("created S3 persistent volume", "name", pvName)
	} else {
		updated := false
		if existingPV.Spec.PersistentVolumeSource.CSI == nil {
			existingPV.Spec.PersistentVolumeSource.CSI = &corev1.CSIPersistentVolumeSource{}
			updated = true
		}
		csiSpec := existingPV.Spec.PersistentVolumeSource.CSI
		if csiSpec.Driver != v1alpha1.S3Driver {
			csiSpec.Driver = v1alpha1.S3Driver
			updated = true
		}
		if csiSpec.VolumeHandle != v1alpha1.S3VolumeHandle {
			csiSpec.VolumeHandle = v1alpha1.S3VolumeHandle
			updated = true
		}
		if csiSpec.VolumeAttributes == nil {
			csiSpec.VolumeAttributes = map[string]string{}
		}
		if csiSpec.VolumeAttributes["bucketName"] != storageSpec.S3.BucketName {
			csiSpec.VolumeAttributes["bucketName"] = storageSpec.S3.BucketName
			updated = true
		}
		if len(existingPV.Spec.AccessModes) != 1 || existingPV.Spec.AccessModes[0] != corev1.ReadWriteMany {
			existingPV.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
			updated = true
		}
		if qty, ok := existingPV.Spec.Capacity[corev1.ResourceStorage]; !ok || qty.Cmp(requestQuantity) != 0 {
			existingPV.Spec.Capacity[corev1.ResourceStorage] = requestQuantity
			updated = true
		}
		if existingPV.Spec.ClaimRef == nil ||
			existingPV.Spec.ClaimRef.Namespace != namespace ||
			existingPV.Spec.ClaimRef.Name != claimName {
			existingPV.Spec.ClaimRef = &corev1.ObjectReference{
				Namespace: namespace,
				Name:      claimName,
			}
			updated = true
		}
		if existingPV.Spec.StorageClassName != "" {
			existingPV.Spec.StorageClassName = ""
			updated = true
		}
		if !equalStringSlice(existingPV.Spec.MountOptions, mountOptions) {
			existingPV.Spec.MountOptions = mountOptions
			updated = true
		}
		if updated {
			if err := r.Update(ctx, existingPV); err != nil {
				return fmt.Errorf("failed to update s3 persistent volume %q: %w", pvName, err)
			}
			log.Infow("updated S3 persistent volume", "name", pvName)
		}
	}

	desiredPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      claimName,
			Namespace: namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: requestQuantity,
				},
			},
			VolumeName: pvName,
		},
	}
	storageClass := ""
	desiredPVC.Spec.StorageClassName = &storageClass

	existingPVC := &corev1.PersistentVolumeClaim{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: claimName}, existingPVC); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		if err := r.Create(ctx, desiredPVC); err != nil {
			return fmt.Errorf("failed to create s3 persistent volume claim %q: %w", claimName, err)
		}
		log.Infow("created S3 persistent volume claim", "name", claimName, "namespace", namespace)
		return nil
	}
	updated := false
	if len(existingPVC.Spec.AccessModes) != 1 || existingPVC.Spec.AccessModes[0] != corev1.ReadWriteMany {
		existingPVC.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
		updated = true
	}
	if existingPVC.Spec.StorageClassName == nil || *existingPVC.Spec.StorageClassName != "" {
		existingPVC.Spec.StorageClassName = &storageClass
		updated = true
	}
	if existingPVC.Spec.VolumeName != pvName {
		existingPVC.Spec.VolumeName = pvName
		updated = true
	}
	if existingPVC.Spec.Resources.Requests == nil {
		existingPVC.Spec.Resources.Requests = corev1.ResourceList{}
	}
	if qty, ok := existingPVC.Spec.Resources.Requests[corev1.ResourceStorage]; !ok || qty.Cmp(requestQuantity) != 0 {
		existingPVC.Spec.Resources.Requests[corev1.ResourceStorage] = requestQuantity
		updated = true
	}
	if updated {
		if err := r.Update(ctx, existingPVC); err != nil {
			return fmt.Errorf("failed to update s3 persistent volume claim %q: %w", claimName, err)
		}
		log.Infow("updated S3 persistent volume claim", "name", claimName, "namespace", namespace)
		return nil
	}
	log.Infof("S3 PVC already configured: %s/%s", namespace, claimName)
	return nil
}
func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
