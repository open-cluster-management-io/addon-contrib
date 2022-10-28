/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openclustermanagementiov1alpha1 "open-cluster-management.io/addon-contrib/example-addon/api/v1alpha1"
)

// HelloSpokeReconciler reconciles a HelloSpoke object
type HelloSpokeReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	localClient kubernetes.Interface
}

//+kubebuilder:rbac:groups=open-cluster-management.io.open-cluster-management.io,resources=hellospokes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=open-cluster-management.io.open-cluster-management.io,resources=hellospokes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=open-cluster-management.io.open-cluster-management.io,resources=hellospokes/finalizers,verbs=update

// Reconcile reads server info from cluster-info configmap and report to hub.
func (r *HelloSpokeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	var obj *openclustermanagementiov1alpha1.HelloSpoke
	err := r.Client.Get(ctx, req.NamespacedName, obj)
	if err != nil {
		return ctrl.Result{}, err
	}

	cm, err := r.localClient.CoreV1().ConfigMaps("kube-public").Get(ctx, "cluster-info", metav1.GetOptions{})
	switch {
	case errors.IsNotFound(err):
		obj.Status.SpokeURL = "NA"
	case err != nil:
		return ctrl.Result{}, err
	}

	if server, ok := cm.Data["server"]; ok {
		obj.Status.SpokeURL = server
	} else {
		obj.Status.SpokeURL = "NA"
	}

	return ctrl.Result{}, r.Client.Update(ctx, obj)
}

// SetupWithManager sets up the controller with the Manager.
func (r *HelloSpokeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&openclustermanagementiov1alpha1.HelloSpoke{}).
		Complete(r)
}
