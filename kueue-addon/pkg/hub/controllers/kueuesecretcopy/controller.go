package kueuesecretcopy

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	informerv1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
	kueueclient "sigs.k8s.io/kueue/client-go/clientset/versioned"
	kueueinformerv1beta1 "sigs.k8s.io/kueue/client-go/informers/externalversions/kueue/v1beta1"

	"open-cluster-management.io/addon-contrib/kueue-addon/pkg/hub/controllers/common"
	clusterinformerv1 "open-cluster-management.io/api/client/cluster/informers/externalversions/cluster/v1"
	clusterlisterv1 "open-cluster-management.io/api/client/cluster/listers/cluster/v1"
	"open-cluster-management.io/sdk-go/pkg/patcher"
)

// kueueSecretCopyController reconciles instances of secret on the hub.
type kueueSecretCopyController struct {
	kubeClient    kubernetes.Interface
	kueueClient   kueueclient.Interface
	clusterLister clusterlisterv1.ManagedClusterLister
	eventRecorder events.Recorder
}

// NewKueueSecretCopyController returns a controller that watches for multikueue secrets in all cluster namespaces
// and ensures a kubeconfig Secret is created/updated in the kueue namespace.
func NewKueueSecretCopyController(
	kubeClient kubernetes.Interface,
	kueueClient kueueclient.Interface,
	secretInformer informerv1.SecretInformer,
	clusterInformer clusterinformerv1.ManagedClusterInformer,
	mkclusterInformer kueueinformerv1beta1.MultiKueueClusterInformer,
	recorder events.Recorder) factory.Controller {
	c := &kueueSecretCopyController{
		kubeClient:    kubeClient,
		kueueClient:   kueueClient,
		clusterLister: clusterInformer.Lister(),
		eventRecorder: recorder.WithComponentSuffix("kueue-secret-copy-controller"),
	}

	return factory.New().
		WithFilteredEventsInformersQueueKeysFunc(
			func(obj runtime.Object) []string {
				accessor, _ := meta.Accessor(obj)
				return []string{fmt.Sprintf("%s/%s", accessor.GetNamespace(), accessor.GetName())}
			},
			func(obj interface{}) bool {
				accessor, _ := meta.Accessor(obj)
				// Filter multikueue secret
				return accessor.GetName() == common.MultiKueueResourceName
			},
			secretInformer.Informer()).
		WithInformersQueueKeysFunc(
			func(obj runtime.Object) []string {
				accessor, _ := meta.Accessor(obj)
				return []string{fmt.Sprintf("%s/%s", accessor.GetName(), common.MultiKueueResourceName)}
			},
			mkclusterInformer.Informer()).
		WithSync(c.sync).
		ToController("KueueSecretCopyController", recorder)
}

// Sync copies the multikueue ServiceAccount secret from the cluster namespace to the kueue namespace as a kubeconfig Secret.
func (c *kueueSecretCopyController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	key := syncCtx.QueueKey()
	logger := klog.FromContext(ctx)
	logger.Info("Reconciling Secret", "key", key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	clusterName := namespace

	origSecret, err := c.kubeClient.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		logger.Info("Secret not found, deleting corresponding kubeconfig secret and MultiKueueCluster", "secret", name, "namespace", namespace)
		// Delete the corresponding kubeconfig secret in kueue namespace when source secret is not found
		kubeconfigSecretName := common.GetMultiKueueSecretName(namespace)
		err = c.kubeClient.CoreV1().Secrets(common.KueueNamespace).Delete(ctx, kubeconfigSecretName, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete kubeconfig secret %s: %v", kubeconfigSecretName, err)
		}
		if err == nil {
			logger.Info("Deleted kubeconfig secret", "secret", kubeconfigSecretName, "namespace", common.KueueNamespace)
		}

		// Delete the corresponding MultiKueueCluster
		err = c.kueueClient.KueueV1beta1().MultiKueueClusters().Delete(ctx, clusterName, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete MultiKueueCluster %s: %v", clusterName, err)
		}
		if err == nil {
			logger.Info("Deleted MultiKueueCluster", "cluster", clusterName)
		}
		return nil
	}
	if err != nil {
		return err
	}

	// Get cluster url from ManagedCluster
	mcl, err := c.clusterLister.Get(clusterName)
	if errors.IsNotFound(err) {
		logger.Info("ManagedCluster not found", "cluster", clusterName)
		return nil
	}
	if err != nil {
		return err
	}

	if len(mcl.Spec.ManagedClusterClientConfigs) == 0 {
		return fmt.Errorf("no client config found for cluster %s", clusterName)
	}
	url := mcl.Spec.ManagedClusterClientConfigs[0].URL

	// Generate the kubeconfig Secret
	kubeconfSecret, err := c.generateKueConfigSecret(origSecret, url, clusterName)
	if err != nil {
		return err
	}
	if kubeconfSecret == nil {
		logger.Info("Kubeconfig secret is not ready")
		return nil
	}

	// generate kubeconfig secret
	_, _, err = resourceapply.ApplySecret(ctx, c.kubeClient.CoreV1(), c.eventRecorder, kubeconfSecret)
	if err != nil {
		return err
	}

	// Create or update MultiKueueCluster for this cluster
	mkCluster := &kueuev1beta1.MultiKueueCluster{
		ObjectMeta: metav1.ObjectMeta{Name: clusterName},
		Spec: kueuev1beta1.MultiKueueClusterSpec{
			KubeConfig: kueuev1beta1.KubeConfig{
				LocationType: kueuev1beta1.SecretLocationType,
				Location:     common.GetMultiKueueSecretName(clusterName),
			},
		},
	}
	err = c.createOrUpdateMultiKueueCluster(ctx, mkCluster)
	return err
}

// generateKueConfigSecret creates a kubeconfig Secret for the given cluster using the original ServiceAccount secret.
func (c *kueueSecretCopyController) generateKueConfigSecret(secret *v1.Secret, clusterAddr, clusterName string) (*v1.Secret, error) {
	saToken, ok := secret.Data["token"]
	if !ok {
		return nil, fmt.Errorf("token not found in secret %s", secret.Name)
	}

	caCert, ok := secret.Data["ca.crt"]
	if !ok {
		return nil, fmt.Errorf("ca.crt not found in secret %s", secret.Name)
	}

	kubeconfigStr := generateKueConfigStr(base64.StdEncoding.EncodeToString(caCert), clusterAddr, clusterName, secret.Name, saToken)

	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.GetMultiKueueSecretName(secret.Namespace),
			Namespace: common.KueueNamespace,
		},
		Data: map[string][]byte{
			"kubeconfig": []byte(kubeconfigStr),
		},
	}, nil
}

// generateKueConfigStr returns a kubeconfig YAML string for the given cluster and ServiceAccount token.
func generateKueConfigStr(caCert, clusterAddr, clusterName, userName string, saToken []byte) string {
	return fmt.Sprintf(`apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: %s
    server: %s
  name: %s
contexts:
- context:
    cluster: %s
    user: %s
  name: %s
current-context: %s
kind: Config
preferences: {}
users:
- name: %s
  user:
    token: %s`,
		caCert, clusterAddr, clusterName, clusterName, userName, clusterName, clusterName, userName, saToken)
}

// createOrUpdateMultiKueueCluster creates or updates a MultiKueueCluster resource for a specific cluster.
func (c *kueueSecretCopyController) createOrUpdateMultiKueueCluster(ctx context.Context, mkc *kueuev1beta1.MultiKueueCluster) error {
	oldmkcluster, err := c.kueueClient.KueueV1beta1().MultiKueueClusters().Get(ctx, mkc.Name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err = c.kueueClient.KueueV1beta1().MultiKueueClusters().Create(ctx, mkc, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}

	mkclusterPatcher := patcher.NewPatcher[*kueuev1beta1.MultiKueueCluster, kueuev1beta1.MultiKueueClusterSpec, kueuev1beta1.MultiKueueClusterStatus](c.kueueClient.KueueV1beta1().MultiKueueClusters())
	_, err = mkclusterPatcher.PatchSpec(ctx, mkc, mkc.Spec, oldmkcluster.Spec)
	return err
}
