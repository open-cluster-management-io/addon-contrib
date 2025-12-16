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
	"net/url"
	"reflect"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	dynamicscoringv1 "open-cluster-management.io/dynamic-scoring/api/v1"
	"open-cluster-management.io/dynamic-scoring/pkg/common"
)

// DynamicScoringConfigReconciler reconciles a DynamicScoringConfig object
type DynamicScoringConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=dynamic-scoring.open-cluster-management.io,resources=dynamicscoringconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dynamic-scoring.open-cluster-management.io,resources=dynamicscoringconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dynamic-scoring.open-cluster-management.io,resources=dynamicscoringconfigs/finalizers,verbs=update
// +kubebuilder:rbac:groups=work.open-cluster-management.io,resources=manifestworks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclusters,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DynamicScoringConfig object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *DynamicScoringConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	klog.Infof("Reconciling DynamicScoringConfig : %s", req.Name)

	var config dynamicscoringv1.DynamicScoringConfig
	if err := r.Get(ctx, req.NamespacedName, &config); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	maskMap := buildMaskMap(config.Spec.Masks)

	var scorerList dynamicscoringv1.DynamicScorerList
	klog.InfoS("Fetching DynamicScorers in namespace", "namespace", req.Namespace, scorerList)
	if err := r.List(ctx, &scorerList, client.InNamespace(req.Namespace)); err != nil {
		klog.ErrorS(err, "Failed to list DynamicScorers", "namespace", req.Namespace)
		return ctrl.Result{}, err
	}

	var clusters clusterv1.ManagedClusterList
	klog.InfoS("Fetching DynamicScorers in namespace", "namespace", req.Namespace, scorerList)
	if err := r.List(ctx, &clusters); err != nil {
		klog.ErrorS(err, "Failed to list ManagedClusters")
		return ctrl.Result{}, err
	}

	clusterToSummaries := buildClusterToSummaries(ctx, clusters.Items, scorerList.Items, maskMap)
	klog.InfoS("Cluster to Summaries Mapping", "mapping", clusterToSummaries)

	for _, cluster := range clusters.Items {
		clusterName := cluster.Name
		currentSummaries := clusterToSummaries[clusterName]
		err := updateConfigManifestWork(ctx, r.Client, clusterName, currentSummaries)
		if err != nil {
			klog.ErrorS(err, "Failed to update ManifestWork for cluster", "cluster", clusterName)
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DynamicScoringConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	mapFn := handler.EnqueueRequestsFromMapFunc(
		func(ctx context.Context, obj client.Object) []ctrl.Request {
			return []ctrl.Request{
				{NamespacedName: client.ObjectKey{
					Name:      common.DynamicScoringConfigName,
					Namespace: obj.GetNamespace(),
				}},
			}
		})

	return ctrl.NewControllerManagedBy(mgr).
		For(&dynamicscoringv1.DynamicScoringConfig{}).
		Named("dynamicscoringconfig").
		Watches(
			&dynamicscoringv1.DynamicScorer{}, // Watch for changes in DynamicScorer
			mapFn,
		).
		Complete(r)
}

func buildMaskMap(masks []common.Mask) map[string]struct{} {
	m := make(map[string]struct{}, len(masks))
	for _, mask := range masks {
		key := mask.ClusterName + "/" + mask.ScoreName
		m[key] = struct{}{}
	}
	return m
}

func isMasked(maskMap map[string]struct{}, clusterName, scoreName string) bool {
	key := clusterName + "/" + scoreName
	_, exists := maskMap[key]
	return exists
}

func isValidURL(s string) bool {
	parsed, err := url.ParseRequestURI(s)
	return err == nil && parsed.Scheme != "" && parsed.Host != ""
}

func buildClusterToSummaries(
	ctx context.Context,
	clusters []clusterv1.ManagedCluster,
	scorers []dynamicscoringv1.DynamicScorer,
	maskMap map[string]struct{},
) map[string][]common.ScorerSummary {
	clusterToSummaries := make(map[string][]common.ScorerSummary)
	for _, cluster := range clusters {
		clusterName := cluster.Name
		currentSummaries := []common.ScorerSummary{}
		for _, scorer := range scorers {
			scoreName, err := getValidScoreName(scorer)
			if err != nil {
				klog.ErrorS(err, "Failed to get valid Score Name", "scorer", scorer.Name)
				continue
			}

			if isMasked(maskMap, clusterName, scoreName) {
				continue
			}

			if scorer.Status.HealthStatus != "Active" {
				continue
			}

			if scorer.Status.LastSyncedConfig == nil && scorer.Spec.ConfigSyncMode == "Full" {
				klog.ErrorS(nil, "LastSyncedConfig is nil for scorer", "scorer", scorer.Name)
				continue
			}

			sourceType, err := getValidSourceType(scorer)
			if err != nil {
				klog.ErrorS(err, "Failed to get valid Source Type", "scorer", scorer.Name)
				continue
			}

			sourceFullEndpoint, err := getValidSourceFullEndpoint(scorer)
			if err != nil {
				klog.ErrorS(err, "Failed to get valid Source Full Endpoint", "scorer", scorer.Name)
				if sourceType != "none" {
					continue
				}
			}
			sourceEndpointAuthName, sourceEndpointAuthKey, err := getValidSourceAuthSecretRef(scorer)
			if err != nil {
				klog.ErrorS(err, "Failed to get valid Source Auth Secret Ref", "scorer", scorer.Name)
				if sourceType != "none" {
					continue
				}
			}

			scoringFullEndpoint, err := getValidScoringFullEndpoint(scorer)
			if err != nil {
				klog.ErrorS(err, "Failed to get valid Source Full Endpoint", "scorer", scorer.Name)
				continue
			}
			scoringEndpointAuthName, scoringEndpointAuthKey, err := getValidScoringAuthSecretRef(scorer)
			if err != nil {
				klog.ErrorS(err, "Failed to get valid Scoring Auth Secret Ref", "scorer", scorer.Name)
				continue
			}

			sourceQuery, err := getValidSourceQuery(scorer)
			if err != nil {
				klog.ErrorS(err, "Failed to get valid Source Query", "scorer", scorer.Name)
				if sourceType != "none" {
					continue
				}
			}

			sourceRange, err := getValidSourceRange(scorer)
			if err != nil {
				klog.ErrorS(err, "Failed to get valid Source Range", "scorer", scorer.Name)
				if sourceType != "none" {
					continue
				}
			}

			sourceStep, err := getValidSourceStep(scorer)
			if err != nil {
				klog.ErrorS(err, "Failed to get valid Source Step", "scorer", scorer.Name)
				if sourceType != "none" {
					continue
				}
			}

			scoringInterval, err := getValidScoringInterval(scorer)
			if err != nil {
				klog.ErrorS(err, "Failed to get valid Scoring Interval", "scorer", scorer.Name)
				continue
			}

			location, err := getValidLocation(scorer)
			if err != nil {
				klog.ErrorS(err, "Failed to get valid Location", "scorer", scorer.Name)
				continue
			}

			scoreDestination, err := getValidScoreDestination(scorer)
			if err != nil {
				klog.ErrorS(err, "Failed to get valid Score Destination", "scorer", scorer.Name)
				continue
			}

			scoreDimensionFormat, err := getValidScoreDimentionFormat(scorer)
			if err != nil {
				klog.ErrorS(err, "Failed to get valid Score Dimension Format", "scorer", scorer.Name)
				continue
			}

			summary := common.ScorerSummary{
				Name:                    scorer.Name,
				ScoreName:               scoreName,
				SourceType:              sourceType,
				SourceEndpoint:          sourceFullEndpoint,
				SourceEndpointAuthName:  sourceEndpointAuthName,
				SourceEndpointAuthKey:   sourceEndpointAuthKey,
				SourceQuery:             sourceQuery,
				SourceRange:             sourceRange,
				SourceStep:              sourceStep,
				ScoringEndpoint:         scoringFullEndpoint,
				ScoringEndpointAuthName: scoringEndpointAuthName,
				ScoringEndpointAuthKey:  scoringEndpointAuthKey,
				ScoringInterval:         scoringInterval,
				Location:                location,
				ScoreDestination:        scoreDestination,
				ScoreDimensionFormat:    scoreDimensionFormat,
			}
			currentSummaries = append(currentSummaries, summary)
			klog.InfoS("Scorer Summary", "summary", summary)
		}
		clusterToSummaries[clusterName] = currentSummaries
	}
	return clusterToSummaries
}

func buildConfigManifestWork(clusterName string, scorerSummaryList []common.ScorerSummary) workv1.ManifestWork {
	summaryJSON, err := json.Marshal(scorerSummaryList)
	if err != nil {
		klog.ErrorS(err, "Failed to marshal summaries", "cluster", clusterName)
		return workv1.ManifestWork{} // Return empty ManifestWork on error
	}

	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.DynamicScoringConfigName,
			Namespace: common.DynamicScoringNamespace,
		},
		Data: map[string]string{
			"summaries": string(summaryJSON),
		},
	}

	manifest := workv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.ManifestWorkConfigMapName,
			Namespace: clusterName,
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: []workv1.Manifest{
					{
						RawExtension: runtime.RawExtension{
							Object: cm,
						},
					},
				},
			},
		},
	}
	return manifest
}

func updateConfigManifestWork(ctx context.Context, c client.Client, clusterName string, currentSummaries []common.ScorerSummary) error {
	manifest := buildConfigManifestWork(clusterName, currentSummaries)
	var existing workv1.ManifestWork
	err := c.Get(ctx, client.ObjectKeyFromObject(&manifest), &existing)
	if errors.IsNotFound(err) {
		if err := c.Create(ctx, &manifest); err != nil {
			klog.ErrorS(err, "Failed to create ManifestWork", "name", manifest.Name, "namespace", manifest.Namespace)
			return err
		}
	} else if err == nil {
		// 差分チェック：既存の summaries と currentSummaries を比較
		var existingCM corev1.ConfigMap
		for _, m := range existing.Spec.Workload.Manifests {
			if err := json.Unmarshal(m.RawExtension.Raw, &existingCM); err == nil && existingCM.Name == common.DynamicScoringConfigName {
				existingSummariesJSON := existingCM.Data["summaries"]
				var existingSummaries []common.ScorerSummary
				if err := json.Unmarshal([]byte(existingSummariesJSON), &existingSummaries); err != nil {
					klog.ErrorS(err, "Failed to unmarshal existing summaries")
					return err
				}

				klog.InfoS("Existing Summaries JSON", "data", existingSummaries)
				klog.InfoS("Current Summaries JSON", "data", currentSummaries)

				if reflect.DeepEqual(existingSummaries, currentSummaries) {
					klog.InfoS("No change in summaries, skipping update", "cluster", clusterName)
					return nil
				}
			}
		}

		// 差分あり → Update
		manifest.ResourceVersion = existing.ResourceVersion
		if err := c.Update(ctx, &manifest); err != nil {
			klog.ErrorS(err, "Failed to update ManifestWork", "name", manifest.Name, "namespace", manifest.Namespace)
			return err
		}
	} else {
		klog.ErrorS(err, "Failed to get ManifestWork", "name", manifest.Name, "namespace", manifest.Namespace)
	}
	return nil
}

func getValidSourceType(scorer dynamicscoringv1.DynamicScorer) (string, error) {
	if scorer.Status.LastSyncedConfig != nil && scorer.Status.LastSyncedConfig.Source.Type != "" {
		return scorer.Status.LastSyncedConfig.Source.Type, nil
	} else if scorer.Spec.Source.Type != "" {
		return scorer.Spec.Source.Type, nil
	}
	return "", fmt.Errorf("failed to fetch valid sourceType")
}

func getValidSourceFullEndpoint(scorer dynamicscoringv1.DynamicScorer) (string, error) {
	parsedConfigURL, err := url.Parse(scorer.Spec.ConfigURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse Config URL: %w", err)
	}

	var sourceHost, sourcePath string
	if scorer.Status.LastSyncedConfig != nil && scorer.Status.LastSyncedConfig.Source.Host != "" {
		sourceHost = scorer.Status.LastSyncedConfig.Source.Host
	} else if scorer.Spec.Source.Host != "" {
		sourceHost = scorer.Spec.Source.Host
	} else {
		sourceHost = parsedConfigURL.Scheme + "://" + parsedConfigURL.Host
	}

	if scorer.Status.LastSyncedConfig != nil && scorer.Status.LastSyncedConfig.Source.Path != "" {
		sourcePath = scorer.Status.LastSyncedConfig.Source.Path
	} else {
		sourcePath = scorer.Spec.Source.Path
	}

	fullEndpoint, err := url.JoinPath(sourceHost, sourcePath)
	if !isValidURL(fullEndpoint) {
		return "", fmt.Errorf("invalid Source Full URL: %s", fullEndpoint)
	}
	return fullEndpoint, nil
}

func getValidScoringFullEndpoint(scorer dynamicscoringv1.DynamicScorer) (string, error) {
	parsedConfigURL, err := url.Parse(scorer.Spec.ConfigURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse Config URL: %w", err)
	}

	var scoringHost, scoringPath string
	if scorer.Status.LastSyncedConfig != nil && scorer.Status.LastSyncedConfig.Scoring.Host != "" {
		scoringHost = scorer.Status.LastSyncedConfig.Source.Host
	} else if scorer.Spec.Scoring.Host != "" {
		scoringHost = scorer.Spec.Scoring.Host
	} else {
		scoringHost = parsedConfigURL.Scheme + "://" + parsedConfigURL.Host
	}

	if scorer.Status.LastSyncedConfig != nil && scorer.Status.LastSyncedConfig.Scoring.Path != "" {
		scoringPath = scorer.Status.LastSyncedConfig.Scoring.Path
	} else {
		scoringPath = scorer.Spec.Scoring.Path
	}

	fullEndpoint, err := url.JoinPath(scoringHost, scoringPath)
	if !isValidURL(fullEndpoint) {
		return "", fmt.Errorf("invalid Scoring Full URL: %s", fullEndpoint)
	}
	return fullEndpoint, nil
}

func getValidSourceAuthSecretRef(scorer dynamicscoringv1.DynamicScorer) (string, string, error) {
	if scorer.Spec.Source.Auth == nil {
		return "", "", nil
	} else if scorer.Spec.Source.Auth.TokenSecretRef.Name == "" {
		return "", "", fmt.Errorf("Source Auth Secret Ref Name is empty")
	} else if scorer.Spec.Source.Auth.TokenSecretRef.Key == "" {
		return "", "", fmt.Errorf("Source Auth Secret Ref Key is empty")
	}
	return scorer.Spec.Source.Auth.TokenSecretRef.Name, scorer.Spec.Source.Auth.TokenSecretRef.Key, nil
}

func getValidScoringAuthSecretRef(scorer dynamicscoringv1.DynamicScorer) (string, string, error) {
	if scorer.Spec.Scoring.Auth == nil {
		return "", "", nil
	} else if scorer.Spec.Scoring.Auth.TokenSecretRef.Name == "" {
		return "", "", fmt.Errorf("Scoring Auth Secret Ref Name is empty")
	} else if scorer.Spec.Scoring.Auth.TokenSecretRef.Key == "" {
		return "", "", fmt.Errorf("Scoring Auth Secret Ref Key is empty")
	}
	return scorer.Spec.Scoring.Auth.TokenSecretRef.Name, scorer.Spec.Scoring.Auth.TokenSecretRef.Key, nil
}

func getValidScoreName(scorer dynamicscoringv1.DynamicScorer) (string, error) {
	if scorer.Status.LastSyncedConfig != nil && scorer.Status.LastSyncedConfig.Scoring.Params.Name != "" {
		return scorer.Status.LastSyncedConfig.Scoring.Params.Name, nil
	} else if scorer.Spec.Scoring.Params != nil && scorer.Spec.Scoring.Params.Name != "" {
		return scorer.Spec.Scoring.Params.Name, nil
	}
	return "", fmt.Errorf("failed to fetch valid scoreName")
}

func getValidSourceQuery(scorer dynamicscoringv1.DynamicScorer) (string, error) {
	if scorer.Status.LastSyncedConfig != nil && scorer.Status.LastSyncedConfig.Source.Params.Query != "" {
		return scorer.Status.LastSyncedConfig.Source.Params.Query, nil
	} else if scorer.Spec.Source.Params != nil && scorer.Spec.Source.Params.Query != "" {
		return scorer.Spec.Source.Params.Query, nil
	}
	return "", fmt.Errorf("failed to fetch valid sourceQuery")
}

func getValidSourceRange(scorer dynamicscoringv1.DynamicScorer) (int, error) {
	if scorer.Status.LastSyncedConfig != nil && scorer.Status.LastSyncedConfig.Source.Params.Range > 0 {
		return scorer.Status.LastSyncedConfig.Source.Params.Range, nil
	} else if scorer.Spec.Source.Params != nil && scorer.Spec.Source.Params.Range > 0 {
		return scorer.Spec.Source.Params.Range, nil
	}
	return 0, fmt.Errorf("failed to fetch valid sourceRange")
}

func getValidSourceStep(scorer dynamicscoringv1.DynamicScorer) (int, error) {
	if scorer.Status.LastSyncedConfig != nil && scorer.Status.LastSyncedConfig.Source.Params.Step > 0 {
		return scorer.Status.LastSyncedConfig.Source.Params.Step, nil
	} else if scorer.Spec.Source.Params != nil && scorer.Spec.Source.Params.Step > 0 {
		return scorer.Spec.Source.Params.Step, nil
	}
	return 0, fmt.Errorf("failed to fetch valid sourceStep")
}

func getValidScoringInterval(scorer dynamicscoringv1.DynamicScorer) (int, error) {
	if scorer.Status.LastSyncedConfig != nil && scorer.Status.LastSyncedConfig.Scoring.Params.Interval > 0 {
		return scorer.Status.LastSyncedConfig.Scoring.Params.Interval, nil
	} else if scorer.Spec.Scoring.Params != nil && scorer.Spec.Scoring.Params.Interval > 0 {
		return scorer.Spec.Scoring.Params.Interval, nil
	}
	return 0, fmt.Errorf("failed to fetch valid scoringInterval")
}

func getValidLocation(scorer dynamicscoringv1.DynamicScorer) (string, error) {
	if scorer.Spec.Location != "" {
		return scorer.Spec.Location, nil
	}
	return "", fmt.Errorf("failed to fetch valid location")
}

func getValidScoreDestination(scorer dynamicscoringv1.DynamicScorer) (string, error) {
	if scorer.Spec.ScoreDestination != "" {
		return scorer.Spec.ScoreDestination, nil
	}
	return "", fmt.Errorf("failed to fetch valid scoreDestination")
}

func getValidScoreDimentionFormat(scorer dynamicscoringv1.DynamicScorer) (string, error) {
	if scorer.Spec.ScoreDimensionFormat != "" {
		return scorer.Spec.ScoreDimensionFormat, nil
	}
	return "${scoreName}", nil
}
