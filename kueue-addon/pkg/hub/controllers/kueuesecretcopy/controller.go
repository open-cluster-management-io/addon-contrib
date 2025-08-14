package kueuesecretcopy

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

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

	"open-cluster-management.io/addon-contrib/kueue-addon/pkg/hub/controllers/common"
	clusterinformerv1 "open-cluster-management.io/api/client/cluster/informers/externalversions/cluster/v1"
	clusterlisterv1 "open-cluster-management.io/api/client/cluster/listers/cluster/v1"
	clusterapiv1 "open-cluster-management.io/api/cluster/v1"
)

// kueueSecretCopyController reconciles instances of secret on the hub.
type kueueSecretCopyController struct {
	kubeClient    kubernetes.Interface
	clusterLister clusterlisterv1.ManagedClusterLister
	eventRecorder events.Recorder
}

// NewKueueSecretCopyController returns a controller that watches for multikueue secrets in all cluster namespaces
// and ensures a kubeconfig Secret is created/updated in the kueue namespace.
func NewKueueSecretCopyController(
	kubeClient kubernetes.Interface,
	secretInformer informerv1.SecretInformer,
	clusterInformer clusterinformerv1.ManagedClusterInformer,
	recorder events.Recorder) factory.Controller {
	c := &kueueSecretCopyController{
		kubeClient:    kubeClient,
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

	clusterSecret, err := c.kubeClient.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		logger.Info("Secret not found, deleting corresponding kubeconfig secret", "secret", name, "namespace", namespace)
		// Delete the corresponding kubeconfig secret in kueue namespace when source secret is not found
		kubeconfigSecretName := common.GetMultiKueueSecretName(namespace)
		err = c.kubeClient.CoreV1().Secrets(common.KueueNamespace).Delete(ctx, kubeconfigSecretName, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete kubeconfig secret %s: %v", kubeconfigSecretName, err)
		}
		if err == nil {
			logger.Info("Deleted kubeconfig secret", "secret", kubeconfigSecretName, "namespace", common.KueueNamespace)
		}
		return nil
	}
	if err != nil {
		return err
	}

	// Get cluster url from ManagedCluster
	clusterName := namespace
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

	clusterUrl := c.getClusterURL(mcl, clusterName)

	// Generate the kubeconfig Secret
	kubeconfSecret, err := c.generateKueConfigSecret(ctx, clusterSecret, clusterUrl, clusterName)
	if err != nil {
		return err
	}
	if kubeconfSecret == nil {
		logger.Info("Kubeconfig secret is not ready")
		return nil
	}

	// generate kubeconfig secret
	_, _, err = resourceapply.ApplySecret(ctx, c.kubeClient.CoreV1(), c.eventRecorder, kubeconfSecret)
	return err
}

// generateKueConfigSecret creates a kubeconfig Secret for the given cluster using the ManagedServiceAccount secret.
func (c *kueueSecretCopyController) generateKueConfigSecret(ctx context.Context, clusterSecret *v1.Secret, clusterAddr, clusterName string) (*v1.Secret, error) {
	clusterToken, ok := clusterSecret.Data["token"]
	if !ok {
		return nil, fmt.Errorf("token not found in secret %s", clusterSecret.Name)
	}

	caCert, err := c.getCACert(ctx, clusterSecret)
	if err != nil {
		return nil, err
	}

	kubeconfigStr := generateKueConfigStr(base64.StdEncoding.EncodeToString(caCert), clusterAddr, clusterName, clusterSecret.Name, clusterToken)

	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.GetMultiKueueSecretName(clusterSecret.Namespace),
			Namespace: common.KueueNamespace,
		},
		Data: map[string][]byte{
			"kubeconfig": []byte(kubeconfigStr),
		},
	}, nil
}

// getClusterURL returns the appropriate cluster URL, using proxy URL if configured.
func (c *kueueSecretCopyController) getClusterURL(mcl *clusterapiv1.ManagedCluster, clusterName string) string {
	url := mcl.Spec.ManagedClusterClientConfigs[0].URL

	if proxyURL := os.Getenv(common.ClusterProxyURLEnv); proxyURL != "" {
		// Ensure proxy URL ends with proper separator
		if proxyURL[len(proxyURL)-1] != '/' {
			proxyURL += "/"
		}
		url = proxyURL + clusterName
	}

	return url
}

// getCACert returns the appropriate CA certificate based on whether cluster proxy is configured.
// When CLUSTER_PROXY_URL is set, it uses the hub CA certificate, otherwise it uses the cluster secret's CA certificate.
func (c *kueueSecretCopyController) getCACert(ctx context.Context, clusterSecret *v1.Secret) ([]byte, error) {
	if os.Getenv(common.ClusterProxyURLEnv) != "" {
		return c.getHubCACert(ctx)
	}

	caCert, ok := clusterSecret.Data["ca.crt"]
	if !ok {
		return nil, fmt.Errorf("ca.crt not found in secret %s", clusterSecret.Name)
	}

	return caCert, nil
}

// getHubCACert retrieves the hub CA certificate from the kube-root-ca.crt ConfigMap
func (c *kueueSecretCopyController) getHubCACert(ctx context.Context) ([]byte, error) {
	hubConfigMap, err := c.kubeClient.CoreV1().ConfigMaps(common.KueueNamespace).Get(ctx, "kube-root-ca.crt", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get hub CA cert from configmap: %v", err)
	}

	caCertData, ok := hubConfigMap.Data["ca.crt"]
	if !ok {
		return nil, fmt.Errorf("ca.crt not found in kube-root-ca.crt configmap")
	}

	return []byte(caCertData), nil
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
