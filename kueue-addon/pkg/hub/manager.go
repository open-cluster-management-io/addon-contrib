package hub

import (
	"context"
	"time"

	"github.com/openshift/library-go/pkg/controller/controllercmd"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"open-cluster-management.io/addon-contrib/kueue-addon/pkg/hub/controllers/admissioncheck"
	"open-cluster-management.io/addon-contrib/kueue-addon/pkg/hub/controllers/kueuesecretcopy"
	clusterclient "open-cluster-management.io/api/client/cluster/clientset/versioned"
	clusterinformers "open-cluster-management.io/api/client/cluster/informers/externalversions"
	kueueclient "sigs.k8s.io/kueue/client-go/clientset/versioned"
	kueueinformers "sigs.k8s.io/kueue/client-go/informers/externalversions"
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

	kueueClient, err := kueueclient.NewForConfig(controllerContext.KubeConfig)
	if err != nil {
		return err
	}

	clusterInformers := clusterinformers.NewSharedInformerFactory(clusterClient, 10*time.Minute)
	kueueInformers := kueueinformers.NewSharedInformerFactory(kueueClient, 10*time.Minute)
	// to reduce cache size if there are larges number of secrets
	secretInformers := kubeinformers.NewSharedInformerFactoryWithOptions(kubeClient, 30*time.Minute, kubeinformers.WithTweakListOptions(
		func(listOptions *metav1.ListOptions) {
			selector := &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "authentication.open-cluster-management.io/is-managed-serviceaccount",
						Operator: metav1.LabelSelectorOpExists,
					},
				},
			}
			listOptions.LabelSelector = metav1.FormatLabelSelector(selector)
		}))

	return RunControllerManagerWithInformers(
		ctx, controllerContext,
		kubeClient, clusterClient, kueueClient, secretInformers,
		clusterInformers, kueueInformers,
	)
}

func RunControllerManagerWithInformers(
	ctx context.Context,
	controllerContext *controllercmd.ControllerContext,
	kubeClient kubernetes.Interface,
	clusterClient clusterclient.Interface,
	kueueClient *kueueclient.Clientset,
	secretInformers kubeinformers.SharedInformerFactory,
	clusterInformers clusterinformers.SharedInformerFactory,
	kueueInformers kueueinformers.SharedInformerFactory,
) error {
	err := kueueInformers.Kueue().V1beta1().AdmissionChecks().Informer().AddIndexers(
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
		kueueInformers.Kueue().V1beta1().AdmissionChecks(),
		controllerContext.EventRecorder,
	)

	kueuesecretcopyController := kueuesecretcopy.NewKueueSecretCopyController(
		kubeClient,
		secretInformers.Core().V1().Secrets(),
		clusterInformers.Cluster().V1().ManagedClusters(),
		controllerContext.EventRecorder,
	)

	go secretInformers.Start(ctx.Done())
	go clusterInformers.Start(ctx.Done())
	go kueueInformers.Start(ctx.Done())

	go admissionCheckController.Run(ctx, 1)
	go kueuesecretcopyController.Run(ctx, 1)

	<-ctx.Done()
	return nil
}
