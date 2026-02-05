package hub

import (
	"context"
	"time"

	"github.com/openshift/library-go/pkg/controller/controllercmd"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	cpclient "sigs.k8s.io/cluster-inventory-api/client/clientset/versioned"
	cpinformers "sigs.k8s.io/cluster-inventory-api/client/informers/externalversions"
	kueueclient "sigs.k8s.io/kueue/client-go/clientset/versioned"
	kueueinformers "sigs.k8s.io/kueue/client-go/informers/externalversions"

	"open-cluster-management.io/addon-contrib/kueue-addon/pkg/hub/controllers/admissioncheck"
	"open-cluster-management.io/addon-contrib/kueue-addon/pkg/hub/controllers/common"
	"open-cluster-management.io/addon-contrib/kueue-addon/pkg/hub/controllers/kueuesecretgen"
	"open-cluster-management.io/addon-contrib/kueue-addon/pkg/hub/controllers/multikueuecluster"
	clusterclient "open-cluster-management.io/api/client/cluster/clientset/versioned"
	clusterinformers "open-cluster-management.io/api/client/cluster/informers/externalversions"
	permissionclientset "open-cluster-management.io/cluster-permission/client/clientset/versioned"
	permissioninformer "open-cluster-management.io/cluster-permission/client/informers/externalversions"
	msacontroller "open-cluster-management.io/managed-serviceaccount/pkg/addon/manager/controller"
	msaclientset "open-cluster-management.io/managed-serviceaccount/pkg/generated/clientset/versioned"
	msainformer "open-cluster-management.io/managed-serviceaccount/pkg/generated/informers/externalversions"
)

// RunControllerManager starts the controllers on hub to manage spoke cluster registration.
func RunControllerManager(ctx context.Context, controllerContext *controllercmd.ControllerContext) error {
	kubeClient, err := kubernetes.NewForConfig(controllerContext.KubeConfig)
	if err != nil {
		return err
	}

	clusterClient, err := clusterclient.NewForConfig(controllerContext.KubeConfig)
	if err != nil {
		return err
	}

	permissionClient, err := permissionclientset.NewForConfig(controllerContext.KubeConfig)
	if err != nil {
		return err
	}

	msaClient, err := msaclientset.NewForConfig(controllerContext.KubeConfig)
	if err != nil {
		return err
	}

	kueueClient, err := kueueclient.NewForConfig(controllerContext.KubeConfig)
	if err != nil {
		return err
	}

	clusterProfileClient, err := cpclient.NewForConfig(controllerContext.KubeConfig)
	if err != nil {
		return err
	}

	clusterInformers := clusterinformers.NewSharedInformerFactory(clusterClient, 10*time.Minute)
	permissionInformers := permissioninformer.NewSharedInformerFactory(permissionClient, 30*time.Minute)
	msaInformers := msainformer.NewSharedInformerFactory(msaClient, 10*time.Minute)
	kueueInformers := kueueinformers.NewSharedInformerFactory(kueueClient, 10*time.Minute)
	cpinformers := cpinformers.NewSharedInformerFactoryWithOptions(clusterProfileClient, 10*time.Minute, cpinformers.WithNamespace(common.KueueNamespace))
	// to reduce cache size if there are larges number of secrets
	secretInformers := kubeinformers.NewSharedInformerFactoryWithOptions(kubeClient, 30*time.Minute, kubeinformers.WithTweakListOptions(
		func(listOptions *metav1.ListOptions) {
			selector := &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      msacontroller.LabelKeySyncedFrom,
						Operator: metav1.LabelSelectorOpExists,
					},
				},
			}
			listOptions.LabelSelector = metav1.FormatLabelSelector(selector)
		}))

	return RunControllerManagerWithInformers(
		ctx, controllerContext,
		kubeClient, clusterClient, permissionClient, msaClient, kueueClient, secretInformers,
		clusterInformers, permissionInformers, msaInformers, kueueInformers, clusterProfileClient, cpinformers,
	)
}

func RunControllerManagerWithInformers(
	ctx context.Context,
	controllerContext *controllercmd.ControllerContext,
	kubeClient kubernetes.Interface,
	clusterClient clusterclient.Interface,
	permissionClient permissionclientset.Interface,
	msaClient msaclientset.Interface,
	kueueClient *kueueclient.Clientset,
	secretInformers kubeinformers.SharedInformerFactory,
	clusterInformers clusterinformers.SharedInformerFactory,
	permissionInformers permissioninformer.SharedInformerFactory,
	msaInformers msainformer.SharedInformerFactory,
	kueueInformers kueueinformers.SharedInformerFactory,
	clusterProfileClient cpclient.Interface,
	cpinformers cpinformers.SharedInformerFactory,
) error {
	err := kueueInformers.Kueue().V1beta2().AdmissionChecks().Informer().AddIndexers(
		cache.Indexers{
			admissioncheck.AdmissionCheckByPlacement: admissioncheck.IndexAdmissionCheckByPlacement,
		})
	if err != nil {
		return err
	}

	admissionCheckController := admissioncheck.NewAdmissionCheckController(
		ctx,
		clusterClient,
		kueueClient,
		clusterInformers.Cluster().V1beta1().Placements(),
		clusterInformers.Cluster().V1beta1().PlacementDecisions(),
		kueueInformers.Kueue().V1beta2().AdmissionChecks(),
		controllerContext.EventRecorder,
	)

	multiKueueClusterController := multikueuecluster.NewMultiKueueClusterController(
		kueueClient,
		cpinformers.Apis().V1alpha1().ClusterProfiles(),
		permissionInformers.Api().V1alpha1().ClusterPermissions(),
		secretInformers.Core().V1().Secrets(),
		kueueInformers.Kueue().V1beta2().MultiKueueClusters(),
		controllerContext.EventRecorder,
	)

	kueuesecretgenController := kueuesecretgen.NewkueueSecretGenController(
		permissionClient,
		msaClient,
		clusterInformers.Cluster().V1().ManagedClusters(),
		permissionInformers.Api().V1alpha1().ClusterPermissions(),
		msaInformers.Authentication().V1beta1().ManagedServiceAccounts(),
		controllerContext.EventRecorder,
	)

	go secretInformers.Start(ctx.Done())
	go clusterInformers.Start(ctx.Done())
	go permissionInformers.Start(ctx.Done())
	go msaInformers.Start(ctx.Done())
	go kueueInformers.Start(ctx.Done())
	go cpinformers.Start(ctx.Done())

	go admissionCheckController.Run(ctx, 1)
	go multiKueueClusterController.Run(ctx, 1)
	go kueuesecretgenController.Run(ctx, 1)
	<-ctx.Done()
	return nil
}
