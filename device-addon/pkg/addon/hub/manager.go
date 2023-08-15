package hub

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"

	"open-cluster-management-io/addon-contrib/device-addon/pkg/apis/v1alpha1"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	deviceaddonclientset "open-cluster-management-io/addon-contrib/device-addon/pkg/client/clientset/versioned"

	"open-cluster-management.io/addon-framework/pkg/addonfactory"
	"open-cluster-management.io/addon-framework/pkg/addonmanager"
	"open-cluster-management.io/addon-framework/pkg/agent"
	"open-cluster-management.io/addon-framework/pkg/utils"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
)

const (
	installationNamespace = "open-cluster-management-agent-addon"
	addonName             = "device-addon"
)

//go:embed manifests/templates
var fs embed.FS

func Run(ctx context.Context, kubeConfig *rest.Config) error {
	addonClient, err := deviceaddonclientset.NewForConfig(kubeConfig)
	if err != nil {
		return err
	}

	mgr, err := addonmanager.New(kubeConfig)
	if err != nil {
		return err
	}

	agentName := utilrand.String(5)

	agentAddon, err := addonfactory.NewAgentAddonFactory(addonName, fs, "manifests/templates").
		WithAgentRegistrationOption(&agent.RegistrationOption{
			CSRConfigurations: agent.KubeClientSignerConfigurations(addonName, agentName),
			CSRApproveCheck:   utils.DefaultCSRApprover(agentName),
			PermissionConfig:  addonRBAC(kubeConfig),
		}).
		WithInstallStrategy(agent.InstallAllStrategy(installationNamespace)).
		WithAgentHealthProber(agentHealthProber()).
		WithConfigGVRs(
			schema.GroupVersionResource{
				Group:    v1alpha1.GroupVersion.Group,
				Version:  v1alpha1.GroupVersion.Version,
				Resource: "deviceaddonconfigs",
			},
		).
		WithGetValuesFuncs(getAddOnConfigFunc(ctx, addonClient)).
		BuildTemplateAgentAddon()
	if err != nil {
		klog.Errorf("failed to build agent %v", err)
		return err
	}

	err = mgr.AddAgent(agentAddon)
	if err != nil {
		klog.Fatal(err)
	}

	err = mgr.Start(ctx)
	if err != nil {
		klog.Fatal(err)
	}
	<-ctx.Done()

	return nil
}

func agentHealthProber() *agent.HealthProber {
	return &agent.HealthProber{
		Type: agent.HealthProberTypeWork,
		WorkProber: &agent.WorkHealthProber{
			ProbeFields: []agent.ProbeField{
				{
					ResourceIdentifier: workv1.ResourceIdentifier{
						Group:     "apps",
						Resource:  "deployments",
						Name:      "device-addon-agent",
						Namespace: installationNamespace,
					},
					ProbeRules: []workv1.FeedbackRule{
						{
							Type: workv1.WellKnownStatusType,
						},
					},
				},
			},
			HealthCheck: func(identifier workv1.ResourceIdentifier, result workv1.StatusFeedbackResult) error {
				if len(result.Values) == 0 {
					return fmt.Errorf("no values are probed for deployment %s/%s", identifier.Namespace, identifier.Name)
				}
				for _, value := range result.Values {
					if value.Name != "ReadyReplicas" {
						continue
					}

					if *value.Value.Integer >= 1 {
						return nil
					}

					return fmt.Errorf("readyReplica is %d for deployment %s/%s", *value.Value.Integer, identifier.Namespace, identifier.Name)
				}
				return fmt.Errorf("readyReplica is not probed")
			},
		},
	}
}

func addonRBAC(kubeConfig *rest.Config) agent.PermissionConfigFunc {
	return func(cluster *clusterv1.ManagedCluster, addon *addonv1alpha1.ManagedClusterAddOn) error {
		kubeClient, err := kubernetes.NewForConfig(kubeConfig)
		if err != nil {
			return err
		}

		groups := agent.DefaultGroups(cluster.Name, addon.Name)

		clusterRole := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("open-cluster-management:%s:agent", addon.Name),
			},
			Rules: []rbacv1.PolicyRule{
				{
					Verbs:     []string{"get", "list", "watch"},
					Resources: []string{"drivers"},
					APIGroups: []string{"edge.open-cluster-management.io"},
				},
				{
					Verbs:     []string{"get", "list", "watch"},
					Resources: []string{"devices"},
					APIGroups: []string{"edge.open-cluster-management.io"},
				},
			},
		}
		_, err = kubeClient.RbacV1().ClusterRoles().Get(context.TODO(), clusterRole.Name, metav1.GetOptions{})
		switch {
		case errors.IsNotFound(err):
			_, createErr := kubeClient.RbacV1().ClusterRoles().Create(context.TODO(), clusterRole, metav1.CreateOptions{})
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
				{
					Verbs:     []string{"get", "list", "watch"},
					Resources: []string{"configmaps"},
					APIGroups: []string{""},
				},
				{
					Verbs:     []string{"get", "list", "watch"},
					Resources: []string{"managedclusteraddons"},
					APIGroups: []string{"addon.open-cluster-management.io"},
				},
				{
					Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
					Resources: []string{"drivers"},
					APIGroups: []string{"edge.open-cluster-management.io"},
				},
				{
					Verbs:     []string{"update", "patch"},
					Resources: []string{"drivers/status"},
					APIGroups: []string{"edge.open-cluster-management.io"},
				},
				{
					Verbs:     []string{"update", "patch"},
					Resources: []string{"drivers/finalizers"},
					APIGroups: []string{"edge.open-cluster-management.io"},
				},
				{
					Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
					Resources: []string{"devices"},
					APIGroups: []string{"edge.open-cluster-management.io"},
				},
				{
					Verbs:     []string{"update", "patch"},
					Resources: []string{"devices/status"},
					APIGroups: []string{"edge.open-cluster-management.io"},
				},
				{
					Verbs:     []string{"update", "patch"},
					Resources: []string{"devices/finalizers"},
					APIGroups: []string{"edge.open-cluster-management.io"},
				},
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

		clusterRoleBinding := &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("open-cluster-management:%s:agent", addon.Name),
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "ClusterRole",
				Name:     fmt.Sprintf("open-cluster-management:%s:agent", addon.Name),
			},
			Subjects: []rbacv1.Subject{
				{Kind: "Group", APIGroup: "rbac.authorization.k8s.io", Name: groups[0]},
			},
		}
		_, err = kubeClient.RbacV1().ClusterRoleBindings().Get(context.TODO(), clusterRoleBinding.Name, metav1.GetOptions{})
		switch {
		case errors.IsNotFound(err):
			_, createErr := kubeClient.RbacV1().ClusterRoleBindings().Create(context.TODO(), clusterRoleBinding, metav1.CreateOptions{})
			if createErr != nil {
				return createErr
			}
		case err != nil:
			return err
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

func getAddOnConfigFunc(ctx context.Context, addonClient deviceaddonclientset.Interface) addonfactory.GetValuesFunc {
	return func(cluster *clusterv1.ManagedCluster, addon *addonv1alpha1.ManagedClusterAddOn) (addonfactory.Values, error) {
		config, err := addonClient.EdgeV1alpha1().DeviceAddOnConfigs(cluster.Name).Get(ctx, addonName, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return map[string]interface{}{}, nil
		}
		if err != nil {
			return nil, err
		}

		data, err := json.Marshal(config.Spec)
		if err != nil {
			return nil, err
		}

		return map[string]interface{}{"AddOnConfigData": string(data)}, nil
	}
}
