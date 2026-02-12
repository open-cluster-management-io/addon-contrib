package kueuesecretgen

import (
	"context"
	"fmt"
	"os"
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"open-cluster-management.io/addon-contrib/kueue-addon/pkg/hub/controllers/common"
	permissionrv1alpha1 "open-cluster-management.io/cluster-permission/api/v1alpha1"
	permissionclientset "open-cluster-management.io/cluster-permission/client/clientset/versioned"
	msav1beta1 "open-cluster-management.io/managed-serviceaccount/apis/authentication/v1beta1"
	msacontroller "open-cluster-management.io/managed-serviceaccount/pkg/addon/manager/controller"
	msaclientset "open-cluster-management.io/managed-serviceaccount/pkg/generated/clientset/versioned"
	"open-cluster-management.io/sdk-go/pkg/patcher"
)

var (
	genericScheme = runtime.NewScheme()
	genericCodec  = serializer.NewCodecFactory(genericScheme).UniversalDeserializer()
)

func init() {
	utilruntime.Must(permissionrv1alpha1.AddToScheme(genericScheme))
}

// applyClusterPermission applies a ClusterPermission from a manifest file
func applyClusterPermission(
	ctx context.Context,
	permissionClient permissionclientset.Interface,
	manifestFunc func(name string) ([]byte, error),
	file string,
	clusterName string) error {

	// Read and decode the manifest
	objBytes, err := manifestFunc(file)
	if err != nil {
		return fmt.Errorf("failed to read manifest %q: %v", file, err)
	}

	obj, _, err := genericCodec.Decode(objBytes, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to decode manifest %q: %v", file, err)
	}

	// Convert to ClusterPermission
	required, ok := obj.(*permissionrv1alpha1.ClusterPermission)
	if !ok {
		return fmt.Errorf("manifest %q is not a ClusterPermission", file)
	}

	// Set the ClusterPermission and subject
	required.Name = common.MultiKueueResourceName
	required.Namespace = clusterName
	if err := setClusterRoleBindingSubject(&required.Spec.ClusterRoleBinding.Subject); err != nil {
		return err
	}

	// Try to get existing ClusterPermission
	existing, err := permissionClient.ApiV1alpha1().ClusterPermissions(clusterName).Get(ctx, required.Name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		// Create if not exists
		_, err := permissionClient.ApiV1alpha1().ClusterPermissions(clusterName).Create(ctx, required, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create ClusterPermission: %v", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get ClusterPermission: %v", err)
	}

	// Update if needed
	patcher := patcher.NewPatcher[
		*permissionrv1alpha1.ClusterPermission, permissionrv1alpha1.ClusterPermissionSpec, permissionrv1alpha1.ClusterPermissionStatus](
		permissionClient.ApiV1alpha1().ClusterPermissions(clusterName))
	_, err = patcher.PatchSpec(ctx, required, required.Spec, existing.Spec)
	return err
}

// applyManagedServiceAccount applies a ManagedServiceAccount
func applyManagedServiceAccount(
	ctx context.Context,
	msaClient msaclientset.Interface,
	clusterName string) error {

	required := &msav1beta1.ManagedServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.MultiKueueResourceName,
			Namespace: clusterName,
			Labels: map[string]string{
				msacontroller.LabelKeyClusterProfileSync: "true",
			},
		},
		Spec: msav1beta1.ManagedServiceAccountSpec{
			Rotation: msav1beta1.ManagedServiceAccountRotation{
				Enabled:  true,
				Validity: metav1.Duration{Duration: time.Hour * 8640},
			},
		},
	}

	// Try to get existing ManagedServiceAccount
	existing, err := msaClient.AuthenticationV1beta1().ManagedServiceAccounts(clusterName).Get(ctx, required.Name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		// Create if not exists
		_, err := msaClient.AuthenticationV1beta1().ManagedServiceAccounts(clusterName).Create(ctx, required, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create ManagedServiceAccount: %v", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get ManagedServiceAccount: %v", err)
	}

	// Update if needed
	patcher := patcher.NewPatcher[
		*msav1beta1.ManagedServiceAccount, msav1beta1.ManagedServiceAccountSpec, msav1beta1.ManagedServiceAccountStatus](
		msaClient.AuthenticationV1beta1().ManagedServiceAccounts(clusterName))
	_, err = patcher.PatchSpec(ctx, required, required.Spec, existing.Spec)
	return err
}

// getPodServiceAccountInfo returns the service account name and namespace from environment variables
func getPodServiceAccountInfo() (name, namespace string, err error) {
	saName := os.Getenv("SERVICE_ACCOUNT_NAME")
	if saName == "" {
		return "", "", fmt.Errorf("SERVICE_ACCOUNT_NAME environment variable is not set")
	}

	saNamespace := os.Getenv("POD_NAMESPACE")
	if saNamespace == "" {
		return "", "", fmt.Errorf("POD_NAMESPACE environment variable is not set")
	}

	return saName, saNamespace, nil
}

// setClusterRoleBindingSubject sets the subject based on impersonation mode
func setClusterRoleBindingSubject(subject *rbacv1.Subject) error {
	if os.Getenv(common.ClusterProxyImpersonationEnv) == "true" {
		// Impersonation mode: bind cluster permission to hub kueue-addon-controller  service account.
		saName, saNamespace, err := getPodServiceAccountInfo()
		if err != nil {
			return fmt.Errorf("failed to get pod service account info for impersonation: %v", err)
		}
		subject.APIGroup = "rbac.authorization.k8s.io"
		subject.Kind = "User"
		subject.Name = fmt.Sprintf("cluster:hub:system:serviceaccount:%s:%s", saNamespace, saName)
		subject.Namespace = ""
	} else {
		// Standard mode: bind cluster permission to managed cluster service account.
		subject.APIGroup = ""
		subject.Kind = "ServiceAccount"
		subject.Name = common.MultiKueueResourceName
		subject.Namespace = "open-cluster-management-agent-addon"
	}
	return nil
}
