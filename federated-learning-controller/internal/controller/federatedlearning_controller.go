/*
Copyright 2024.

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

package controller

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	flv1alpha1 "github/open-cluster-management/federated-learning/api/v1alpha1"
	"github/open-cluster-management/federated-learning/internal/logger"
)

var log = logger.DefaultZapLogger()

const (
	FederatedLearningFinalizer     = "federated-learning.open-cluster-management.io/resource-cleanup"
	MessageWaitingReady            = "Awaiting server and client readiness"
	MessageWaitingAvailableClients = "Expected %d clusters, but only %d meet the criteria"
	MessageRunning                 = "Assigned %d clusters for client execution in model training"
	MessageCompleted               = "Model training successful. Check storage for details"
)

// FederatedLearningReconciler reconciles a FederatedLearning object
type FederatedLearningReconciler struct {
	ctrl.Manager
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=federation-ai.open-cluster-management.io,resources=federatedlearnings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=federation-ai.open-cluster-management.io,resources=federatedlearnings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=federation-ai.open-cluster-management.io,resources=federatedlearnings/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;delete

// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *FederatedLearningReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	instance := &flv1alpha1.FederatedLearning{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	if errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}

	defer func() {
		if err != nil {
			instance.Status.Message = err.Error()
			instance.Status.Phase = flv1alpha1.PhaseFailed
		}
		if e := r.Status().Update(ctx, instance); e != nil {
			log.Errorf("failed to update the instance phase into failed: %v", e)
		}
	}()

	// deleting the instance, clean up the resources with finalizer
	if instance.DeletionTimestamp != nil {
		if err := r.pruneClientResources(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
		if controllerutil.RemoveFinalizer(instance, FederatedLearningFinalizer) {
			if err = r.Update(ctx, instance); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// add finalizer
	if controllerutil.AddFinalizer(instance, FederatedLearningFinalizer) {
		if err = r.Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
	}

	if instance.Status.Phase == flv1alpha1.PhaseFailed {
		log.Infof("FederatedLearning %s is %s", instance.Name, instance.Status.Phase)
		return ctrl.Result{}, nil
	}

	// Initialize phase with Waiting
	if instance.Status.Phase == "" {
		instance.Status.Phase = flv1alpha1.PhaseWaiting
		instance.Status.Message = MessageWaitingReady
		if e := r.Status().Update(ctx, instance); e != nil {
			return ctrl.Result{}, e
		}
	}

	// Waiting -> Running
	if instance.Status.Phase == flv1alpha1.PhaseWaiting || instance.Status.Phase == flv1alpha1.PhaseRunning {
		// 1. server: storage, job (rounds, minAvailableClients)
		if err := r.federatedLearningServer(ctx, instance); err != nil {
			log.Errorf("failed to create/update the server: %v", err)
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

		// 2. client: placement(based on selected cluster -> Running), Running -> generate manifestwork
		if err := r.federatedLearningClient(ctx, instance); err != nil {
			log.Errorf("failed to create/update the clients: %v", err)
			return ctrl.Result{}, err
		}
	}

	// Running -> Completed
	if instance.Status.Phase == flv1alpha1.PhaseRunning ||
		instance.Status.Phase == flv1alpha1.PhaseCompleted {
		job := &batchv1.Job{}
		err = r.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: getSeverName(instance.Name)}, job)
		if err != nil {
			return ctrl.Result{}, err
		}

		if job.Status.Succeeded > 0 && MessageCompleted != instance.Status.Message {
			log.Info("the job has been completed")
			instance.Status.Phase = flv1alpha1.PhaseCompleted
			instance.Status.Message = MessageCompleted
			if err = r.Status().Update(ctx, instance); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	// requeue if the phase is Waiting or Running
	if instance.Status.Phase == flv1alpha1.PhaseRunning || instance.Status.Phase == flv1alpha1.PhaseWaiting {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// TODO: enhance it
func getDirFile(modelPath string) (dir, file string, err error) {
	// Check if the path is empty
	if modelPath == "" {
		return "", "", fmt.Errorf("path cannot be empty")
	}

	// Get the base (file or dir) and the directory part
	dir = path.Dir(modelPath)
	file = path.Base(modelPath)

	// If the file contains a dot, treat it as a file, otherwise it's a directory
	if strings.Contains(file, ".") {
		// It's a file
		return dir, file, nil
	}

	// It's a directory, return empty string for file
	return dir, "", nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *FederatedLearningReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&flv1alpha1.FederatedLearning{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return true
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return true
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
			},
		}).
		Named("federatedlearning").
		Complete(r)
}
