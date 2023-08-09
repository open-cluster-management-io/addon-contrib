package addon

import (
	"context"
	"embed"
	"fmt"
	"os"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"open-cluster-management.io/addon-contrib/clusternet-addon/pkg/helpers"

	"open-cluster-management.io/addon-framework/pkg/addonfactory"
	"open-cluster-management.io/addon-framework/pkg/agent"
	"open-cluster-management.io/addon-framework/pkg/utils"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

const defaultImage = "quay.io/skeeey/clusternet-addon:latest"

//go:embed manifests
var FS embed.FS

func NewRegistrationOption(ctx context.Context, kubeConfig *rest.Config, addonName, agentName string) *agent.RegistrationOption {
	return &agent.RegistrationOption{
		CSRConfigurations: agent.KubeClientSignerConfigurations(addonName, agentName),
		CSRApproveCheck:   utils.DefaultCSRApprover(agentName),
		PermissionConfig:  addonRBAC(ctx, kubeConfig),
		Namespace:         helpers.DefaultInstallationNamespace,
	}
}

func GetDefaultValues(cluster *clusterv1.ManagedCluster, addon *addonapiv1alpha1.ManagedClusterAddOn) (addonfactory.Values, error) {
	installNamespace := addon.Spec.InstallNamespace
	if len(installNamespace) == 0 {
		installNamespace = helpers.DefaultInstallationNamespace
	}
	image := os.Getenv("IMAGE_NAME")
	if len(image) == 0 {
		image = defaultImage
	}

	manifestConfig := struct {
		KubeConfigSecret      string
		ClusterName           string
		AddonInstallNamespace string
		Image                 string
	}{
		KubeConfigSecret:      fmt.Sprintf("%s-hub-kubeconfig", addon.Name),
		AddonInstallNamespace: installNamespace,
		ClusterName:           cluster.Name,
		Image:                 image,
	}

	return addonfactory.StructToValues(manifestConfig), nil
}

func addonRBAC(ctx context.Context, kubeConfig *rest.Config) agent.PermissionConfigFunc {
	return func(cluster *clusterv1.ManagedCluster, addon *addonapiv1alpha1.ManagedClusterAddOn) error {
		kubeClient, err := kubernetes.NewForConfig(kubeConfig)
		if err != nil {
			return err
		}

		groups := agent.DefaultGroups(cluster.Name, addon.Name)

		clusterRole := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("open-cluster-management:clusternet-addon:%s", cluster.Name),
			},
			Rules: []rbacv1.PolicyRule{
				{Verbs: []string{"*"}, ResourceNames: []string{cluster.Name}, Resources: []string{"sockets"}, APIGroups: []string{"proxies.clusternet.io"}},
			},
		}

		clusterRoleBinding := &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("open-cluster-management:clusternet-addon:%s", cluster.Name),
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "ClusterRole",
				Name:     clusterRole.Name,
			},
			Subjects: []rbacv1.Subject{
				{Kind: "Group", APIGroup: "rbac.authorization.k8s.io", Name: groups[0]},
			},
		}

		_, err = kubeClient.RbacV1().ClusterRoles().Get(ctx, clusterRole.Name, metav1.GetOptions{})
		switch {
		case errors.IsNotFound(err):
			_, createErr := kubeClient.RbacV1().ClusterRoles().Create(ctx, clusterRole, metav1.CreateOptions{})
			if createErr != nil {
				return createErr
			}
		case err != nil:
			return err
		}

		_, err = kubeClient.RbacV1().ClusterRoleBindings().Get(ctx, clusterRoleBinding.Name, metav1.GetOptions{})
		switch {
		case errors.IsNotFound(err):
			_, createErr := kubeClient.RbacV1().ClusterRoleBindings().Create(ctx, clusterRoleBinding, metav1.CreateOptions{})
			if createErr != nil {
				return createErr
			}
		case err != nil:
			return err
		}

		role := &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("open-cluster-management:%s:agent", addon.Name),
				Namespace: cluster.Name,
			},
			Rules: []rbacv1.PolicyRule{
				{Verbs: []string{"get", "list", "watch"}, Resources: []string{"configmaps"}, APIGroups: []string{""}},
				{Verbs: []string{"get", "list", "watch"}, Resources: []string{"managedclusteraddons"}, APIGroups: []string{"addon.open-cluster-management.io"}},
			},
		}

		binding := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("open-cluster-management:%s:agent", addon.Name),
				Namespace: cluster.Name,
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Role",
				Name:     fmt.Sprintf("open-cluster-management:%s:agent", addon.Name),
			},
			Subjects: []rbacv1.Subject{
				{Kind: "Group", APIGroup: "rbac.authorization.k8s.io", Name: groups[0]},
			},
		}

		_, err = kubeClient.RbacV1().Roles(cluster.Name).Get(context.TODO(), role.Name, metav1.GetOptions{})
		switch {
		case errors.IsNotFound(err):
			_, createErr := kubeClient.RbacV1().Roles(cluster.Name).Create(context.TODO(), role, metav1.CreateOptions{})
			if createErr != nil {
				return createErr
			}
		case err != nil:
			return err
		}

		_, err = kubeClient.RbacV1().RoleBindings(cluster.Name).Get(context.TODO(), binding.Name, metav1.GetOptions{})
		switch {
		case errors.IsNotFound(err):
			_, createErr := kubeClient.RbacV1().RoleBindings(cluster.Name).Create(context.TODO(), binding, metav1.CreateOptions{})
			if createErr != nil {
				return createErr
			}
		case err != nil:
			return err
		}

		return nil
	}
}
