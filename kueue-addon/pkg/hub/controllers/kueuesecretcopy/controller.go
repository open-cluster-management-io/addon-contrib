package kueuesecretcopy

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

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
	kueuev1beta2 "sigs.k8s.io/kueue/apis/kueue/v1beta2"
	kueueclient "sigs.k8s.io/kueue/client-go/clientset/versioned"
	kueueinformerv1beta2 "sigs.k8s.io/kueue/client-go/informers/externalversions/kueue/v1beta2"

	"open-cluster-management.io/addon-contrib/kueue-addon/pkg/hub/controllers/common"
	clusterinformerv1 "open-cluster-management.io/api/client/cluster/informers/externalversions/cluster/v1"
	clusterlisterv1 "open-cluster-management.io/api/client/cluster/listers/cluster/v1"
	permissioninformer "open-cluster-management.io/cluster-permission/client/informers/externalversions/api/v1alpha1"
	permissionlisterv1alpha1 "open-cluster-management.io/cluster-permission/client/listers/api/v1alpha1"
	"open-cluster-management.io/sdk-go/pkg/patcher"
)

// kueueSecretCopyController reconciles instances of secret on the hub.
type kueueSecretCopyController struct {
	kubeClient       kubernetes.Interface
	kueueClient      kueueclient.Interface
	clusterLister    clusterlisterv1.ManagedClusterLister
	permissionLister permissionlisterv1alpha1.ClusterPermissionLister
	eventRecorder    events.Recorder
}

// NewKueueSecretCopyController returns a controller that watches for multikueue secrets in all cluster namespaces
// and ensures a kubeconfig Secret is created/updated in the kueue namespace.
func NewKueueSecretCopyController(
	kubeClient kubernetes.Interface,
	kueueClient kueueclient.Interface,
	secretInformer informerv1.SecretInformer,
	clusterInformer clusterinformerv1.ManagedClusterInformer,
	permissionInformers permissioninformer.ClusterPermissionInformer,
	mkclusterInformer kueueinformerv1beta2.MultiKueueClusterInformer,
	recorder events.Recorder) factory.Controller {
	c := &kueueSecretCopyController{
		kubeClient:       kubeClient,
		kueueClient:      kueueClient,
		clusterLister:    clusterInformer.Lister(),
		permissionLister: permissionInformers.Lister(),
		eventRecorder:    recorder.WithComponentSuffix("kueue-secret-copy-controller"),
	}

	factory := factory.New().
		// watch MultiKueueCluster we create
		WithInformersQueueKeysFunc(
			func(obj runtime.Object) []string {
				accessor, _ := meta.Accessor(obj)
				return []string{fmt.Sprintf("%s/%s", accessor.GetName(), common.MultiKueueResourceName)}
			},
			mkclusterInformer.Informer()).
		// watch clusterpermision
		WithInformersQueueKeysFunc(
			func(obj runtime.Object) []string {
				accessor, _ := meta.Accessor(obj)
				return []string{fmt.Sprintf("%s/%s", accessor.GetNamespace(), common.MultiKueueResourceName)}
			},
			permissionInformers.Informer())

	if !common.IsImpersonationMode() {
		// Non-impersonation mode: also watch MSA secrets
		factory = factory.WithFilteredEventsInformersQueueKeysFunc(
			func(obj runtime.Object) []string {
				accessor, _ := meta.Accessor(obj)
				return []string{fmt.Sprintf("%s/%s", accessor.GetNamespace(), accessor.GetName())}
			},
			func(obj any) bool {
				accessor, _ := meta.Accessor(obj)
				return accessor.GetName() == common.MultiKueueResourceName
			},
			secretInformer.Informer())
	}

	return factory.WithSync(c.sync).ToController("KueueSecretCopyController", recorder)
}

// sync reconciles kubeconfig secrets and MultiKueueCluster resources based on cluster state
func (c *kueueSecretCopyController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	key := syncCtx.QueueKey()
	logger := klog.FromContext(ctx)
	logger.Info("Reconciling", "key", key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	clusterName := namespace

	// Check if cluster resources should be cleaned up
	shouldCleanup, err := c.shouldCleanupResources(ctx, clusterName, namespace, name)
	if err != nil {
		return err
	}
	if shouldCleanup {
		logger.Info("Cluster resources should be cleaned up", "cluster", clusterName)
		return c.cleanupResources(ctx, clusterName)
	}

	// Create/update kubeconfig secret
	err = c.createOrUpdateKubeconfigSecret(ctx, clusterName)
	if err != nil {
		return err
	}

	// Create/update MultiKueueCluster
	return c.createOrUpdateMultiKueueCluster(ctx, clusterName)
}

// shouldCleanupResources determines if cluster resources should be cleaned up
// Returns true if cluster permissions or MSA secrets are missing
func (c *kueueSecretCopyController) shouldCleanupResources(ctx context.Context, clusterName, secretNamespace, secretName string) (bool, error) {
	logger := klog.FromContext(ctx)

	_, err := c.permissionLister.ClusterPermissions(clusterName).Get(common.MultiKueueResourceName)
	if errors.IsNotFound(err) {
		logger.Info("ClusterPermission not found, resources should be cleaned up", "cluster", clusterName)
		return true, nil
	}

	if !common.IsImpersonationMode() {
		_, err = c.kubeClient.CoreV1().Secrets(secretNamespace).Get(ctx, secretName, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			logger.Info("MSA Secret not found, resources should be cleaned up", "secret", secretName, "namespace", secretNamespace)
			return true, nil
		}
	}

	return false, nil
}

// cleanupResources removes kubeconfig secret and MultiKueueCluster
func (c *kueueSecretCopyController) cleanupResources(ctx context.Context, clusterName string) error {
	logger := klog.FromContext(ctx)

	kubeconfigSecretName := common.GetMultiKueueSecretName(clusterName)
	err := c.kubeClient.CoreV1().Secrets(common.KueueNamespace).Delete(ctx, kubeconfigSecretName, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete kubeconfig secret %s: %v", kubeconfigSecretName, err)
	}
	if err == nil {
		logger.Info("Deleted kubeconfig secret", "secret", kubeconfigSecretName, "namespace", common.KueueNamespace)
	}

	err = c.kueueClient.KueueV1beta2().MultiKueueClusters().Delete(ctx, clusterName, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete MultiKueueCluster %s: %v", clusterName, err)
	}
	if err == nil {
		logger.Info("Deleted MultiKueueCluster", "cluster", clusterName)
	}

	return nil
}

func (c *kueueSecretCopyController) createOrUpdateKubeconfigSecret(ctx context.Context, clusterName string) error {
	clusterURL, err := c.getClusterURL(clusterName)
	if err != nil {
		return err
	}

	kubeconfigSecret, err := c.generateKubeconfigSecret(ctx, clusterName, clusterURL)
	if err != nil {
		return err
	}

	_, _, err = resourceapply.ApplySecret(ctx, c.kubeClient.CoreV1(), c.eventRecorder, kubeconfigSecret)
	return err
}

func (c *kueueSecretCopyController) getClusterURL(clusterName string) (string, error) {
	if proxyURL := os.Getenv(common.ClusterProxyURLEnv); proxyURL != "" {
		if !strings.HasSuffix(proxyURL, "/") {
			proxyURL += "/"
		}
		return proxyURL + clusterName, nil
	}

	cluster, err := c.clusterLister.Get(clusterName)
	if err != nil {
		return "", fmt.Errorf("failed to get ManagedCluster %s: %v", clusterName, err)
	}

	if len(cluster.Spec.ManagedClusterClientConfigs) == 0 {
		return "", fmt.Errorf("no client config found for cluster %s", clusterName)
	}

	return cluster.Spec.ManagedClusterClientConfigs[0].URL, nil
}

// generateKubeconfigSecret creates a kubeconfig Secret for the given cluster using the appropriate token.
func (c *kueueSecretCopyController) generateKubeconfigSecret(ctx context.Context, clusterName, clusterURL string) (*v1.Secret, error) {
	if common.IsImpersonationMode() {
		return c.buildImpersonationKubeconfigSecret(ctx, clusterName, clusterURL)
	}

	if os.Getenv(common.ClusterProxyURLEnv) != "" {
		return c.buildProxyKubeconfigSecret(ctx, clusterName, clusterURL)
	}

	return c.buildStandardKubeconfigSecret(ctx, clusterName, clusterURL)
}

func (c *kueueSecretCopyController) buildImpersonationKubeconfigSecret(ctx context.Context, clusterName, clusterURL string) (*v1.Secret, error) {
	clusterToken, err := c.getHubServiceAccountToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get hub service account token for impersonation: %v", err)
	}

	caCert, err := c.getHubCACert(ctx)
	if err != nil {
		return nil, err
	}

	return c.buildKubeconfigSecret(clusterName, clusterURL, "kueue-addon-controller", clusterToken, caCert), nil
}

func (c *kueueSecretCopyController) buildProxyKubeconfigSecret(ctx context.Context, clusterName, clusterURL string) (*v1.Secret, error) {
	clusterSecret, err := c.getClusterSecret(ctx, clusterName)
	if err != nil {
		return nil, err
	}

	clusterToken, ok := clusterSecret.Data["token"]
	if !ok {
		return nil, fmt.Errorf("token not found in secret %s", clusterSecret.Name)
	}

	caCert, err := c.getHubCACert(ctx)
	if err != nil {
		return nil, err
	}

	return c.buildKubeconfigSecret(clusterName, clusterURL, clusterSecret.Name, clusterToken, caCert), nil
}

func (c *kueueSecretCopyController) buildStandardKubeconfigSecret(ctx context.Context, clusterName, clusterURL string) (*v1.Secret, error) {
	clusterSecret, err := c.getClusterSecret(ctx, clusterName)
	if err != nil {
		return nil, err
	}

	clusterToken, ok := clusterSecret.Data["token"]
	if !ok {
		return nil, fmt.Errorf("token not found in secret %s", clusterSecret.Name)
	}

	caCert, ok := clusterSecret.Data["ca.crt"]
	if !ok {
		return nil, fmt.Errorf("ca.crt not found in secret %s", clusterSecret.Name)
	}

	return c.buildKubeconfigSecret(clusterName, clusterURL, clusterSecret.Name, clusterToken, caCert), nil
}

func (c *kueueSecretCopyController) getClusterSecret(ctx context.Context, clusterName string) (*v1.Secret, error) {
	clusterSecret, err := c.kubeClient.CoreV1().Secrets(clusterName).Get(ctx, common.MultiKueueResourceName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster secret for %s: %v", clusterName, err)
	}
	return clusterSecret, nil
}

func (c *kueueSecretCopyController) buildKubeconfigSecret(clusterName, clusterURL, userName string, token, caCert []byte) *v1.Secret {
	kubeconfigStr := generateKueConfigStr(base64.StdEncoding.EncodeToString(caCert), clusterURL, clusterName, userName, token)

	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.GetMultiKueueSecretName(clusterName),
			Namespace: common.KueueNamespace,
		},
		Data: map[string][]byte{
			"kubeconfig": []byte(kubeconfigStr),
		},
	}
}

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

func (c *kueueSecretCopyController) getHubServiceAccountToken() ([]byte, error) {
	tokenPath := "/var/run/secrets/kubernetes.io/serviceaccount/token"
	token, err := os.ReadFile(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read service account token from %s: %v", tokenPath, err)
	}
	return token, nil
}

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

func (c *kueueSecretCopyController) createOrUpdateMultiKueueCluster(ctx context.Context, clusterName string) error {
	mkCluster := &kueuev1beta2.MultiKueueCluster{
		ObjectMeta: metav1.ObjectMeta{Name: clusterName},
		Spec: kueuev1beta2.MultiKueueClusterSpec{
			ClusterSource: kueuev1beta2.ClusterSource{
				KubeConfig: &kueuev1beta2.KubeConfig{
					LocationType: kueuev1beta2.SecretLocationType,
					Location:     common.GetMultiKueueSecretName(clusterName),
				},
			},
		},
	}

	oldmkcluster, err := c.kueueClient.KueueV1beta2().MultiKueueClusters().Get(ctx, mkCluster.Name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err = c.kueueClient.KueueV1beta2().MultiKueueClusters().Create(ctx, mkCluster, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}

	mkclusterPatcher := patcher.NewPatcher[*kueuev1beta2.MultiKueueCluster, kueuev1beta2.MultiKueueClusterSpec, kueuev1beta2.MultiKueueClusterStatus](c.kueueClient.KueueV1beta2().MultiKueueClusters())
	_, err = mkclusterPatcher.PatchSpec(ctx, mkCluster, mkCluster.Spec, oldmkcluster.Spec)
	return err
}
