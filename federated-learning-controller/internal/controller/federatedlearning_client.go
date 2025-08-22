package controller

import (
	"context"
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterclient "open-cluster-management.io/api/client/cluster/clientset/versioned"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	clusterv1beta2 "open-cluster-management.io/api/cluster/v1beta2"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github/open-cluster-management/federated-learning/api/v1alpha1"
	flv1alpha1 "github/open-cluster-management/federated-learning/api/v1alpha1"
	"github/open-cluster-management/federated-learning/internal/controller/manifests"
	"github/open-cluster-management/federated-learning/internal/controller/manifests/applier"
)

var clusterclientset *clusterclient.Clientset

// +kubebuilder:rbac:groups=policy.open-cluster-management.io,resources=placementbindings,verbs=get;update;watch;list;delete;create
// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=placementdecisions,verbs=get;update;watch;list;delete;create
// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=placements,verbs=get;update;watch;list;delete;create
// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclusters,verbs=get;update;watch;list
// +kubebuilder:rbac:groups=work.open-cluster-management.io,resources=manifestworks,verbs=get;list;watch;update;delete;create
// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclustersetbindings;managedclustersets;managedclustersets/bind;managedclustersets/finalizers;managedclustersets/join,verbs=create;get;list;patch;update;watch;delete

func (r *FederatedLearningReconciler) federatedLearningClient(ctx context.Context,
	instance *flv1alpha1.FederatedLearning,
) (err error) {
	// delete the placement and manifestwork of it
	if instance.DeletionTimestamp != nil {
		return nil
	}

	// generate placement
	err = r.deployPlacement(ctx, instance)
	if err != nil {
		return err
	}

	placement := &clusterv1beta1.Placement{
		ObjectMeta: metav1.ObjectMeta{
			Name: instance.Name, Namespace: instance.Namespace,
		},
	}
	if err := r.Get(ctx, client.ObjectKeyFromObject(placement), placement); err != nil {
		return err
	}

	// try to switch the phase from waiting into running
	if err = r.toRunning(ctx, instance, placement); err != nil {
		log.Error(err)
		return err
	}
	return nil
}

// Determine dataKey based on the placement(and instance), and render the workload from the decisions
func (r *FederatedLearningReconciler) generateWorkload(ctx context.Context, instance *flv1alpha1.FederatedLearning,
	placement *clusterv1beta1.Placement,
) error {
	log.Info("generate the workload for the selected clusters")
	// TODO: provide a reasonable way to determine the data configuration
	dataKey := ""
	for _, predicate := range placement.Spec.Predicates {
		for _, matchExpression := range predicate.RequiredClusterSelector.ClaimSelector.MatchExpressions {
			if matchExpression.Operator == metav1.LabelSelectorOpExists {
				dataKey = matchExpression.Key
			}
		}
	}
	log.Infow("determine the dataKey", "dataKey", dataKey)
	count := 0
	for _, decisionGroup := range placement.Status.DecisionGroups {
		for _, decisionName := range decisionGroup.Decisions {
			decision := clusterv1beta1.PlacementDecision{}
			if err := r.Get(ctx, types.NamespacedName{
				Namespace: instance.Namespace, Name: decisionName,
			}, &decision); err != nil {
				return err
			}
			// generate workload
			for _, clusterDecision := range decision.Status.Decisions {
				log.Infow("generate the workload for the cluster", "cluster", clusterDecision.ClusterName)
				cluster := &clusterv1.ManagedCluster{}
				if err := r.Get(ctx, types.NamespacedName{Name: clusterDecision.ClusterName}, cluster); err != nil {
					return err
				}
				dataConfig := ""
				for _, clusterClaim := range cluster.Status.ClusterClaims {
					if dataKey == clusterClaim.Name {
						dataConfig = clusterClaim.Value
					}
				}
				if dataConfig == "" {
					return fmt.Errorf("failed to the dataConfig(%s) from cluster(%s)", dataKey, cluster.Name)
				}
				if err := r.clusterWorkload(ctx, instance, cluster.Name, dataConfig); err != nil {
					log.Errorw("failed to generate the workload for the cluster", "cluster", cluster.Name, "error", err)
					return err
				}
				count++
			}

		}
		message := fmt.Sprintf("applied %d manifestworks to the clusters", count)
		if instance.Status.Phase == flv1alpha1.PhaseRunning && instance.Status.Message != message {
			instance.Status.Phase = flv1alpha1.PhaseRunning
			instance.Status.Message = message
			if err := r.Status().Update(ctx, instance); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *FederatedLearningReconciler) pruneClientResources(ctx context.Context, instance *flv1alpha1.FederatedLearning,
) (err error) {
	placement := &clusterv1beta1.Placement{
		ObjectMeta: metav1.ObjectMeta{
			Name: instance.Name, Namespace: instance.Namespace,
		},
	}
	if err = r.Get(ctx, client.ObjectKeyFromObject(placement), placement); err != nil && !errors.IsNotFound(err) {
		return err
	}
	if errors.IsNotFound(err) {
		return nil
	}

	// delete the mainfestwork based on the placement status
	// TODO: Use the Label to delete it
	for _, decisionGroup := range placement.Status.DecisionGroups {
		for _, decisionName := range decisionGroup.Decisions {
			decision := clusterv1beta1.PlacementDecision{}
			if err := r.Get(ctx, types.NamespacedName{
				Namespace: instance.Namespace, Name: decisionName,
			}, &decision); err != nil {
				return err
			}
			// generate workload
			for _, clusterDecision := range decision.Status.Decisions {
				namesapce := clusterDecision.ClusterName
				work := &workv1.ManifestWork{}
				log.Infow("delete the workload for the cluster", "cluster", namesapce)
				err = r.Get(ctx, types.NamespacedName{Namespace: namesapce, Name: instance.Name}, work)
				if err != nil && !errors.IsNotFound(err) {
					return err
				} else if errors.IsNotFound(err) {
					continue
				}
				if err = r.Delete(ctx, work); err != nil {
					return err
				}
			}
		}
	}

	if err = r.Delete(ctx, placement); err != nil {
		return err
	}

	return nil
}

func (r *FederatedLearningReconciler) clusterWorkload(ctx context.Context, instance *flv1alpha1.FederatedLearning,
	clusterName, dataConfig string,
) error {
	serverAddress := ""
	for _, listener := range instance.Status.Listeners {
		serverAddress = listener.Address
	}
	if serverAddress == "" {
		return fmt.Errorf("wait the server address to be ready!")
	}

	clientParams := &manifests.FederatedLearningClientParams{
		ManifestName:       instance.Name,
		ManifestNamespace:  clusterName,
		ClientJobNamespace: instance.Namespace,
		ClientJobName:      fmt.Sprintf("%s-client", instance.Name),
		ClientJobImage:     instance.Spec.Client.Image,
		ClientDataConfig:   dataConfig,
		ServerAddress:      serverAddress,
		ObsSidecarImage:    instance.ObjectMeta.Annotations[v1alpha1.AnnotationSidecarImage],
	}

	render, deployer := applier.NewRenderer(manifests.ClientFiles), applier.NewDeployer(r.Client)
	unstructuredObjects, err := render.Render("client", "", func(profile string) (interface{}, error) {
		return clientParams, nil
	})
	if err != nil {
		return err
	}
	for _, obj := range unstructuredObjects {
		if err := deployer.Deploy(obj); err != nil {
			return err
		}
	}
	return nil
}

// 1. If selectedClusters < minimizeClients, then requeue and update waiting message
// 2. Else switch to running
func (r *FederatedLearningReconciler) toRunning(ctx context.Context, instance *flv1alpha1.FederatedLearning,
	placement *clusterv1beta1.Placement,
) error {
	selectedClusters := placement.Status.NumberOfSelectedClusters
	minimizeClients := instance.Spec.Server.MinAvailableClients
	if selectedClusters < int32(minimizeClients) {
		log.Infow("waiting for the available clients", "selected", selectedClusters, "minimize", minimizeClients)
		message := fmt.Sprintf(MessageWaitingAvailableClients, minimizeClients, selectedClusters)
		if message != instance.Status.Message {
			instance.Status.Message = message
			if err := r.Client.Status().Update(ctx, instance); err != nil {
				log.Error(err)
				return err
			}
		}
		return nil
	}

	// generate manifestwork for the selected cluster
	if err := r.generateWorkload(ctx, instance, placement); err != nil {
		return err
	}

	// switch into running
	message := fmt.Sprintf(MessageRunning, selectedClusters)
	if instance.Status.Phase != flv1alpha1.PhaseRunning || instance.Status.Message != message {
		log.Infow("switch to Running", "message", message)
		instance.Status.Phase = flv1alpha1.PhaseRunning
		instance.Status.Message = message
		if err := r.Client.Status().Update(ctx, instance); err != nil {
			return err
		}
	}
	return nil
}

func (r *FederatedLearningReconciler) deployPlacement(ctx context.Context,
	instance *flv1alpha1.FederatedLearning,
) error {
	// placement
	expectedPlacement := &clusterv1beta1.Placement{
		ObjectMeta: metav1.ObjectMeta{
			Name: instance.Name, Namespace: instance.Namespace,
		},
		Spec: instance.Spec.Client.Placement,
	}
	// for namespaced resource, set ownerreference of controller
	if err := controllerutil.SetControllerReference(instance, expectedPlacement, r.GetScheme()); err != nil {
		return err
	}

	// Attempt to get the existing Placement
	existingPlacement := &clusterv1beta1.Placement{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(expectedPlacement), existingPlacement); err != nil {
		if errors.IsNotFound(err) {
			// Create the Placement if it does not exist

			log.Info("create the placement")
			if err := r.Create(ctx, expectedPlacement); err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		// If the Placement exists but differs, update it
		if !reflect.DeepEqual(existingPlacement.Spec, expectedPlacement.Spec) {
			existingPlacement.Spec = expectedPlacement.Spec
			log.Info("update the placement for clients")
			if err := r.Update(ctx, existingPlacement); err != nil {
				return err
			}
		}
	}

	// managedclustersetbinding
	if clusterclientset == nil {
		var err error
		clusterclientset, err = clusterclient.NewForConfig(r.GetConfig())
		if err != nil {
			return err
		}
	}

	for _, clusterSet := range instance.Spec.Client.Placement.ClusterSets {
		expectedClusterSetBinding := &clusterv1beta2.ManagedClusterSetBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterSet,
				Namespace: instance.Namespace,
			},
			Spec: clusterv1beta2.ManagedClusterSetBindingSpec{
				ClusterSet: clusterSet,
			},
		}

		// Attempt to get the existing Placement
		existingClusterSetBinding, err := clusterclientset.ClusterV1beta2().ManagedClusterSetBindings(instance.Namespace).Get(ctx, clusterSet, metav1.GetOptions{})

		if err != nil {
			if errors.IsNotFound(err) {
				log.Info("create the clustersetbinding for clients")
				_, err = clusterclientset.ClusterV1beta2().ManagedClusterSetBindings(instance.Namespace).Create(
					ctx, expectedClusterSetBinding, metav1.CreateOptions{})
				if err != nil {
					return err
				}
			} else {
				return err
			}
		} else {
			// If the Placement exists but differs, update it
			if !reflect.DeepEqual(existingClusterSetBinding.Spec, expectedClusterSetBinding.Spec) {
				existingClusterSetBinding.Spec = expectedClusterSetBinding.Spec
				log.Info("update the clustersetbinding for clients")
				_, err = clusterclientset.ClusterV1beta2().ManagedClusterSetBindings(instance.Namespace).Update(ctx, existingClusterSetBinding, metav1.UpdateOptions{})
				if err != nil {
					return err
				}
			}
		}
	}

	log.Info("placement and clustersetbinding are ready")
	return nil
}
