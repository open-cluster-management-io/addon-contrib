/*
Copyright 2025.

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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dynamicscoringv1alpha1 "open-cluster-management.io/dynamic-scoring/api/v1alpha1"
	"open-cluster-management.io/dynamic-scoring/pkg/common"
)

// DynamicScorerReconciler reconciles a DynamicScorer object
type DynamicScorerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=dynamic-scoring.open-cluster-management.io,resources=dynamicscorers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dynamic-scoring.open-cluster-management.io,resources=dynamicscorers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dynamic-scoring.open-cluster-management.io,resources=dynamicscorers/finalizers,verbs=update

func (r *DynamicScorerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	klog.Info("Reconciling DynamicScorer: ", req.Name)

	var dynamicscorer dynamicscoringv1alpha1.DynamicScorer
	if err := r.Get(ctx, req.NamespacedName, &dynamicscorer); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if err := r.Status().Update(ctx, &dynamicscorer); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DynamicScorerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&dynamicscoringv1alpha1.DynamicScorer{}).
		Named("dynamicscorer").
		Complete(r); err != nil {
		return err
	}

	go func() {
		<-mgr.Elected()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		// start periodic health check
		startPeriodicHealthCheck(ctx, r.Client, common.DynamicScorerHealthCheckInterval*time.Second)
	}()

	return nil
}

// syncScoringHealthz checks the /healthz endpoint of the scoring service
// host is extracted from the ConfigURL specified in the DynamicScorer spec
func syncScoringHealthz(ctx context.Context, dynamicscorer *dynamicscoringv1alpha1.DynamicScorer) error {

	klog.Infof("Checking scoring healthz for %s", dynamicscorer.Name)
	parsedURL, err := url.Parse(dynamicscorer.Spec.ConfigURL)

	if err != nil {
		dynamicscorer.Status.HealthStatus = common.ScorerHealthStatusInactive
		klog.Errorf("Failed to parse URL: %v", err)
		return err
	}

	scoringHealthzURL := fmt.Sprintf("%s://%s/healthz", parsedURL.Scheme, parsedURL.Host)

	resp, err := http.Get(scoringHealthzURL)
	if err != nil {
		dynamicscorer.Status.HealthStatus = common.ScorerHealthStatusInactive
		klog.Errorf("Request failed: %v", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		dynamicscorer.Status.HealthStatus = common.ScorerHealthStatusActive
	} else {
		dynamicscorer.Status.HealthStatus = common.ScorerHealthStatusInactive
	}

	return nil
}

func syncScoringConfig(ctx context.Context, dynamicscorer *dynamicscoringv1alpha1.DynamicScorer) error {

	klog.Infof("Sync config for %s", dynamicscorer.Name)

	resp, err := http.Get(dynamicscorer.Spec.ConfigURL)

	if err != nil {
		dynamicscorer.Status.HealthStatus = common.ScorerHealthStatusInactive
		klog.Errorf("Request failed: %v", err)
		return err
	}
	defer resp.Body.Close()

	var newConfig common.Config

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		dynamicscorer.Status.HealthStatus = common.ScorerHealthStatusInactive
		return err
	}

	if err := json.Unmarshal(body, &newConfig); err != nil {
		dynamicscorer.Status.HealthStatus = common.ScorerHealthStatusInactive
		return err
	}

	dynamicscorer.Status.HealthStatus = common.ScorerHealthStatusActive
	dynamicscorer.Status.LastSyncedConfig = &newConfig

	return nil
}

// startPeriodicHealthCheck starts a goroutine that periodically checks the health of all DynamicScorers
// and syncs their config if ConfigSyncMode is set to "Full"
func startPeriodicHealthCheck(ctx context.Context, c client.Client, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	klog.Info("Start periodic health check!!")

	for {
		select {
		case <-ticker.C:
			var list dynamicscoringv1alpha1.DynamicScorerList
			if err := c.List(ctx, &list); err != nil {
				klog.Errorf("Failed to list DynamicScorers %v", err)
				continue
			}

			for _, scorer := range list.Items {
				s := scorer // avoid pointer issues in loop
				originalStatus := s.Status.DeepCopy()

				if s.Spec.ConfigSyncMode == common.ConfigSyncModeFull { // "Full"
					err := syncScoringConfig(ctx, &s)
					if err != nil {
						klog.Errorf("Failed to sync config %s", s.Name)
					}
					if !reflect.DeepEqual(originalStatus, &s.Status) {
						if err := c.Status().Update(ctx, &s); err != nil {
							klog.Errorf("Failed to update DynamicScorer config %s", s.Name)
						} else {
							klog.Infof("Updated DynamicScorer status %s", s.Name)
						}
					} else {
						klog.Infof("No changes in DynamicScorer %s status", s.Name)
					}
				} else { // ConfigSyncModeNone "None", just check healthz
					err := syncScoringHealthz(ctx, &s)
					if err != nil {
						klog.Errorf("Health check failed %s", s.Name)
					}
					if originalStatus.HealthStatus != s.Status.HealthStatus {
						if err := c.Status().Update(ctx, &s); err != nil {
							klog.Errorf("Failed to update DynamicScorer status %s", s.Name)
						} else {
							klog.Infof("Updated DynamicScorer %s", s.Name)
						}
					} else {
						klog.Infof("No changes in DynamicScorer %s status", s.Name)
					}
				}
			}
		case <-ctx.Done():
			return
		}
	}
}
