package applier

import (
	"context"
	"encoding/json"
	"strings"

	"maps"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Deployer is the interface for the kubernetes resource deployer
type Deployer interface {
	Deploy(unsObj *unstructured.Unstructured) error
}

type deployFunc func(*unstructured.Unstructured, *unstructured.Unstructured) error

// deployer is an implementation of Deployer interface
type deployer struct {
	client      client.Client
	deployFuncs map[string]deployFunc
}

// NewDeployer creates a new HoHDeployer
func NewDeployer(client client.Client) Deployer {
	deployer := &deployer{client: client}
	deployer.deployFuncs = map[string]deployFunc{
		"Deployment":         deployer.deployDeployment,
		"StatefulSet":        deployer.deployDeployment,
		"Job":                deployer.deployJob,
		"Service":            deployer.deployService,
		"ServiceAccount":     deployer.deployServiceAccount,
		"ConfigMap":          deployer.deployConfigMap,
		"Secret":             deployer.deploySecret,
		"Role":               deployer.deployRole,
		"RoleBinding":        deployer.deployRoleBinding,
		"ClusterRole":        deployer.deployClusterRole,
		"ClusterRoleBinding": deployer.deployClusterRoleBinding,
		"PodMonitor":         deployer.deployPodMonitor,
		"ManifestWork":       deployer.deployDeployment,
	}
	return deployer
}

func (d *deployer) Deploy(unsObj *unstructured.Unstructured) error {
	foundObj := &unstructured.Unstructured{}
	foundObj.SetGroupVersionKind(unsObj.GetObjectKind().GroupVersionKind())
	err := d.client.Get(
		context.TODO(),
		types.NamespacedName{Name: unsObj.GetName(), Namespace: unsObj.GetNamespace()},
		foundObj,
	)
	if err != nil {
		if errors.IsNotFound(err) {
			return d.client.Create(context.TODO(), unsObj)
		}
		return err
	}

	// if resource has annotation skip-creation-if-exist: true, then it will not be updated
	metadata, ok := unsObj.Object["metadata"].(map[string]interface{})
	if ok {
		annotations, ok := metadata["annotations"].(map[string]interface{})
		if ok && annotations != nil && annotations["skip-creation-if-exist"] != nil {
			if strings.ToLower(annotations["skip-creation-if-exist"].(string)) == "true" {
				return nil
			}
		}
	}

	deployFunction, ok := d.deployFuncs[foundObj.GetKind()]
	if ok {
		return deployFunction(unsObj, foundObj)
	} else {
		if !apiequality.Semantic.DeepDerivative(unsObj, foundObj) {
			unsObj.SetGroupVersionKind(unsObj.GetObjectKind().GroupVersionKind())
			unsObj.SetResourceVersion(foundObj.GetResourceVersion())
			return d.client.Update(context.TODO(), unsObj)
		}
	}

	return nil
}

func (d *deployer) deployDeployment(desiredObj, existingObj *unstructured.Unstructured) error {
	// should not use DeepDerivative for typed object due to https://github.com/kubernetes/apimachinery/issues/110
	if !apiequality.Semantic.DeepDerivative(desiredObj.Object["spec"], existingObj.Object["spec"]) ||
		!apiequality.Semantic.DeepDerivative(desiredObj.GetLabels(), existingObj.GetLabels()) ||
		!apiequality.Semantic.DeepDerivative(desiredObj.GetAnnotations(), existingObj.GetAnnotations()) {
		desiredObj.SetResourceVersion(existingObj.GetResourceVersion())
		return d.client.Update(context.TODO(), desiredObj)
	}
	return nil
}

// deployJob handles the immutability constraints of batchv1.Job.
// If there are meaningful differences between the desired and existing Job specs (ignoring
// system-injected labels on the pod template), we delete the existing Job and let the next
// reconcile create a fresh one, instead of attempting an Update that would violate immutability.
func (d *deployer) deployJob(desiredObj, existingObj *unstructured.Unstructured) error {
	// Unmarshal into typed Jobs
	existingJSON, _ := existingObj.MarshalJSON()
	existingJob := &batchv1.Job{}
	if err := json.Unmarshal(existingJSON, existingJob); err != nil {
		return err
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredJob := &batchv1.Job{}
	if err := json.Unmarshal(desiredJSON, desiredJob); err != nil {
		return err
	}

	// Normalize: ignore system-injected labels on pod template that cause false diffs
	stripSystemLabels := func(m map[string]string) map[string]string {
		if m == nil {
			return nil
		}
		c := make(map[string]string, len(m))
		maps.Copy(c, m)
		delete(c, "controller-uid")
		delete(c, "batch.kubernetes.io/controller-uid")
		delete(c, "batch.kubernetes.io/job-name")
		return c
	}

	// Make shallow copies of label maps before mutation to avoid unintended side effects
	existingJob.Spec.Template.Labels = stripSystemLabels(existingJob.Spec.Template.Labels)
	desiredJob.Spec.Template.Labels = stripSystemLabels(desiredJob.Spec.Template.Labels)

	// Compare desired vs existing. If equal, do nothing. If different, delete existing Job.
	specsEqual := apiequality.Semantic.DeepDerivative(desiredJob.Spec, existingJob.Spec)
	labelsEqual := apiequality.Semantic.DeepDerivative(desiredJob.Labels, existingJob.Labels)
	annotationsEqual := apiequality.Semantic.DeepDerivative(desiredJob.Annotations, existingJob.Annotations)

	if specsEqual && labelsEqual && annotationsEqual {
		return nil
	}

	// Delete and let next reconcile recreate to satisfy immutability constraints
	propagation := metav1.DeletePropagationForeground
	return d.client.Delete(context.TODO(), existingJob, &client.DeleteOptions{PropagationPolicy: &propagation})
}

func (d *deployer) deployService(desiredObj, existingObj *unstructured.Unstructured) error {
	existingJSON, _ := existingObj.MarshalJSON()
	existingService := &corev1.Service{}
	err := json.Unmarshal(existingJSON, existingService)
	if err != nil {
		return err
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredService := &corev1.Service{}
	err = json.Unmarshal(desiredJSON, desiredService)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredService.Spec, existingService.Spec) ||
		!apiequality.Semantic.DeepDerivative(desiredService.GetLabels(), existingService.GetLabels()) ||
		!apiequality.Semantic.DeepDerivative(desiredService.GetAnnotations(), existingService.GetAnnotations()) {
		desiredService.ObjectMeta.ResourceVersion = existingService.ObjectMeta.ResourceVersion
		desiredService.Spec.ClusterIP = existingService.Spec.ClusterIP
		return d.client.Update(context.TODO(), desiredService)
	}

	return nil
}

func (d *deployer) deployServiceAccount(desiredObj, existingObj *unstructured.Unstructured) error {
	existingJSON, _ := existingObj.MarshalJSON()
	existingSA := &corev1.ServiceAccount{}
	err := json.Unmarshal(existingJSON, existingSA)
	if err != nil {
		return err
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredSA := &corev1.ServiceAccount{}
	err = json.Unmarshal(desiredJSON, desiredSA)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredSA.Secrets, existingSA.Secrets) ||
		!apiequality.Semantic.DeepDerivative(desiredSA.ImagePullSecrets, existingSA.ImagePullSecrets) ||
		!apiequality.Semantic.DeepDerivative(desiredSA.GetLabels(), existingSA.GetLabels()) ||
		!apiequality.Semantic.DeepDerivative(desiredSA.GetAnnotations(), existingSA.GetAnnotations()) {
		return d.client.Update(context.TODO(), desiredSA)
	}

	return nil
}

func (d *deployer) deployPodMonitor(desiredObj, existingObj *unstructured.Unstructured) error {
	existingJSON, _ := existingObj.MarshalJSON()
	existingMonitor := &monitoringv1.PodMonitor{}
	err := json.Unmarshal(existingJSON, existingMonitor)
	if err != nil {
		return err
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredMonitor := &monitoringv1.PodMonitor{}
	err = json.Unmarshal(desiredJSON, desiredMonitor)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredMonitor.Spec, existingMonitor.Spec) ||
		!apiequality.Semantic.DeepDerivative(desiredMonitor.GetLabels(), existingMonitor.GetLabels()) ||
		!apiequality.Semantic.DeepDerivative(desiredMonitor.GetAnnotations(), existingMonitor.GetAnnotations()) {
		desiredMonitor.ObjectMeta.ResourceVersion = existingMonitor.ObjectMeta.ResourceVersion
		return d.client.Update(context.TODO(), desiredMonitor)
	}

	return nil
}

func (d *deployer) deployConfigMap(desiredObj, existingObj *unstructured.Unstructured) error {
	existingJSON, _ := existingObj.MarshalJSON()
	existingConfigMap := &corev1.ConfigMap{}
	err := json.Unmarshal(existingJSON, existingConfigMap)
	if err != nil {
		return err
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredConfigMap := &corev1.ConfigMap{}
	err = json.Unmarshal(desiredJSON, desiredConfigMap)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredConfigMap.Data, existingConfigMap.Data) ||
		!apiequality.Semantic.DeepDerivative(desiredConfigMap.GetLabels(), existingConfigMap.GetLabels()) ||
		!apiequality.Semantic.DeepDerivative(desiredConfigMap.GetAnnotations(), existingConfigMap.GetAnnotations()) {
		return d.client.Update(context.TODO(), desiredConfigMap)
	}

	return nil
}

func (d *deployer) deploySecret(desiredObj, existingObj *unstructured.Unstructured) error {
	existingJSON, _ := existingObj.MarshalJSON()
	existingSecret := &corev1.Secret{}
	err := json.Unmarshal(existingJSON, existingSecret)
	if err != nil {
		return err
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredSecret := &corev1.Secret{}
	err = json.Unmarshal(desiredJSON, desiredSecret)
	if err != nil {
		return err
	}

	// handle secret stringData and data
	existingStrData := map[string]string{}
	for key, value := range existingSecret.Data {
		existingStrData[key] = string(value)
	}

	if !apiequality.Semantic.DeepDerivative(desiredSecret.StringData, existingStrData) ||
		!apiequality.Semantic.DeepDerivative(desiredSecret.Data, existingSecret.Data) ||
		!apiequality.Semantic.DeepDerivative(desiredSecret.GetLabels(), existingSecret.GetLabels()) ||
		!apiequality.Semantic.DeepDerivative(desiredSecret.GetAnnotations(), existingSecret.GetAnnotations()) {
		return d.client.Update(context.TODO(), desiredSecret)
	}

	return nil
}

func (d *deployer) deployRole(desiredObj, existingObj *unstructured.Unstructured) error {
	existingJSON, _ := existingObj.MarshalJSON()
	existingRole := &rbacv1.Role{}
	err := json.Unmarshal(existingJSON, existingRole)
	if err != nil {
		return err
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredRole := &rbacv1.Role{}
	err = json.Unmarshal(desiredJSON, desiredRole)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredRole.Rules, existingRole.Rules) ||
		!apiequality.Semantic.DeepDerivative(desiredRole.GetLabels(), existingRole.GetLabels()) ||
		!apiequality.Semantic.DeepDerivative(desiredRole.GetAnnotations(), existingRole.GetAnnotations()) {
		return d.client.Update(context.TODO(), desiredRole)
	}

	return nil
}

func (d *deployer) deployRoleBinding(desiredObj, existingObj *unstructured.Unstructured) error {
	existingJSON, _ := existingObj.MarshalJSON()
	existingRB := &rbacv1.RoleBinding{}
	err := json.Unmarshal(existingJSON, existingRB)
	if err != nil {
		return err
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredRB := &rbacv1.RoleBinding{}
	err = json.Unmarshal(desiredJSON, desiredRB)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredRB.Subjects, existingRB.Subjects) ||
		!apiequality.Semantic.DeepDerivative(desiredRB.RoleRef, existingRB.RoleRef) ||
		!apiequality.Semantic.DeepDerivative(desiredRB.GetLabels(), existingRB.GetLabels()) ||
		!apiequality.Semantic.DeepDerivative(desiredRB.GetAnnotations(), existingRB.GetAnnotations()) {
		return d.client.Update(context.TODO(), desiredRB)
	}

	return nil
}

func (d *deployer) deployClusterRole(desiredObj, existingObj *unstructured.Unstructured) error {
	existingJSON, _ := existingObj.MarshalJSON()
	existingCB := &rbacv1.ClusterRole{}
	err := json.Unmarshal(existingJSON, existingCB)
	if err != nil {
		return err
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredCB := &rbacv1.ClusterRole{}
	err = json.Unmarshal(desiredJSON, desiredCB)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredCB.Rules, existingCB.Rules) ||
		!apiequality.Semantic.DeepDerivative(desiredCB.AggregationRule, existingCB.AggregationRule) ||
		!apiequality.Semantic.DeepDerivative(desiredCB.GetLabels(), existingCB.GetLabels()) ||
		!apiequality.Semantic.DeepDerivative(desiredCB.GetAnnotations(), existingCB.GetAnnotations()) {
		return d.client.Update(context.TODO(), desiredCB)
	}

	return nil
}

func (d *deployer) deployClusterRoleBinding(desiredObj, existingObj *unstructured.Unstructured) error {
	existingJSON, _ := existingObj.MarshalJSON()
	existingCRB := &rbacv1.ClusterRoleBinding{}
	err := json.Unmarshal(existingJSON, existingCRB)
	if err != nil {
		return err
	}

	desiredJSON, _ := desiredObj.MarshalJSON()
	desiredCRB := &rbacv1.ClusterRoleBinding{}
	err = json.Unmarshal(desiredJSON, desiredCRB)
	if err != nil {
		return err
	}

	if !apiequality.Semantic.DeepDerivative(desiredCRB.Subjects, existingCRB.Subjects) ||
		!apiequality.Semantic.DeepDerivative(desiredCRB.RoleRef, existingCRB.RoleRef) ||
		!apiequality.Semantic.DeepDerivative(desiredCRB.GetLabels(), existingCRB.GetLabels()) ||
		!apiequality.Semantic.DeepDerivative(desiredCRB.GetAnnotations(), existingCRB.GetAnnotations()) {
		return d.client.Update(context.TODO(), desiredCRB)
	}

	return nil
}
