package kueuesecretcopy

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	v1 "k8s.io/api/core/v1"
	informerv1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"

	"open-cluster-management.io/addon-contrib/kueue-addon/pkg/hub/controllers/common"
	clusterinformerv1 "open-cluster-management.io/api/client/cluster/informers/externalversions/cluster/v1"
	clusterlisterv1 "open-cluster-management.io/api/client/cluster/listers/cluster/v1"
)

// kueueSecretCopyController reconciles instances of secret on the hub.
type kueueSecretCopyController struct {
	kubeClient    kubernetes.Interface
	clusterLister clusterlisterv1.ManagedClusterLister
	eventRecorder events.Recorder
}

// NewKueueSecretCopyController creates a new controller that copies secrets from cluster namespace to kueue namespace
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
				// filter multikueue secret
				return strings.Contains(accessor.GetName(), common.MultiKueueSecretPrefix)
			},
			secretInformer.Informer()).
		WithSync(c.sync).
		ToController("KueueSecretCopyController", recorder)
}

func (c *kueueSecretCopyController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	key := syncCtx.QueueKey()
	logger := klog.FromContext(ctx)
	logger.Info("Reconciling Secret", "key", key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	secret, err := c.kubeClient.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		logger.Info("secret not found", "secret", name, "namespace", namespace)
		return nil
	}
	if err != nil {
		return err
	}

	// get cluster url
	clusterName := namespace
	mcl, err := c.clusterLister.Get(clusterName)
	if errors.IsNotFound(err) {
		logger.Info("managed cluster not found", "cluster", clusterName)
		return nil
	}
	if err != nil {
		return err
	}

	if len(mcl.Spec.ManagedClusterClientConfigs) == 0 {
		return fmt.Errorf("no client config found for cluster %s", clusterName)
	}
	url := mcl.Spec.ManagedClusterClientConfigs[0].URL

	// generate kubeconfig secret
	kubeconfSecret, err := c.generateKueConfigSecret(secret, url, name)
	if err != nil {
		return err
	}
	if kubeconfSecret == nil {
		logger.Info("kubeconfig secret is not ready")
		return nil
	}

	return c.createOrUpdateSecret(ctx, kubeconfSecret)
}

func (c *kueueSecretCopyController) createOrUpdateSecret(ctx context.Context, secret *v1.Secret) error {
	existSecret, err := c.kubeClient.CoreV1().Secrets(secret.Namespace).Get(ctx, secret.Name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err := c.kubeClient.CoreV1().Secrets(secret.Namespace).Create(ctx, secret, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}

	newSecret := existSecret.DeepCopy()
	newSecret.Data = secret.Data
	_, err = c.kubeClient.CoreV1().Secrets(secret.Namespace).Update(ctx, newSecret, metav1.UpdateOptions{})
	return err
}

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

	// Create the Secret containing kubeconfig
	kubeconfSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secret.Name,
			Namespace: common.KueueNamespace,
		},
		Data: map[string][]byte{
			"kubeconfig": []byte(kubeconfigStr),
		},
	}

	return kubeconfSecret, nil
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
