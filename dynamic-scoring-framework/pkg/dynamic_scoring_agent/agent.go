package dynamic_scoring_agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	corev1informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	cmdfactory "open-cluster-management.io/addon-framework/pkg/cmd/factory"
	"open-cluster-management.io/addon-framework/pkg/lease"
	"open-cluster-management.io/addon-framework/pkg/version"
	addonv1alpha1client "open-cluster-management.io/api/client/addon/clientset/versioned"
	clientset "open-cluster-management.io/api/client/cluster/clientset/versioned"
	"open-cluster-management.io/api/client/cluster/listers/cluster/v1alpha1"
	apiv1alpha2 "open-cluster-management.io/api/cluster/v1alpha1"
	"open-cluster-management.io/dynamic-scoring/pkg/common"
	"open-cluster-management.io/sdk-go/pkg/basecontroller/factory"
)

var (
	dynamicScoreGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: common.DynamicScoreName,
			Help: "Dynamic score from each scorer",
		},
		[]string{
			common.DynamicScoreLabelPrefix + "cluster",
			common.DynamicScoreLabelPrefix + "node",
			common.DynamicScoreLabelPrefix + "device",
			common.DynamicScoreLabelPrefix + "namespace",
			common.DynamicScoreLabelPrefix + "app",
			common.DynamicScoreLabelPrefix + "pod",
			common.DynamicScoreLabelPrefix + "container",
			common.DynamicScoreLabelPrefix + "meta",
			common.DynamicScoreLabelPrefix + "score_name",
		},
	)
)

func init() {
	prometheus.MustRegister(dynamicScoreGauge)
}

func startMetricsServer() {
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		klog.Info("Starting metrics server on :8080")
		if err := http.ListenAndServe(":8080", nil); err != nil {
			klog.Fatalf("failed to start metrics server: %v", err)
		}
	}()
}

func NewAgentCommand(addonName string) *cobra.Command {
	o := NewAgentOptions(addonName)
	cmd := cmdfactory.
		NewControllerCommandConfig("dynamic-scoring-addon-agent", version.Get(), o.RunAgent).
		NewCommand()
	cmd.Use = "agent"
	cmd.Short = "Start the addon agent"

	o.AddFlags(cmd)
	return cmd
}

type AgentOptions struct {
	HubKubeconfigFile     string
	ManagedKubeconfigFile string
	SpokeClusterName      string
	AddonName             string
	AddonNamespace        string
}

func NewAgentOptions(addonName string) *AgentOptions {
	return &AgentOptions{AddonName: addonName}
}

func (o *AgentOptions) AddFlags(cmd *cobra.Command) {
	flags := cmd.Flags()
	flags.StringVar(&o.HubKubeconfigFile, "hub-kubeconfig", o.HubKubeconfigFile,
		"Location of kubeconfig file to connect to hub cluster.")
	flags.StringVar(&o.ManagedKubeconfigFile, "managed-kubeconfig", o.ManagedKubeconfigFile,
		"Location of kubeconfig file to connect to the managed cluster.")
	flags.StringVar(&o.SpokeClusterName, "cluster-name", o.SpokeClusterName, "Name of spoke cluster.")
	flags.StringVar(&o.AddonNamespace, "addon-namespace", o.AddonNamespace, "Installation namespace of addon.")
	flags.StringVar(&o.AddonName, "addon-name", o.AddonName, "name of the addon.")
}

func (o *AgentOptions) RunAgent(ctx context.Context, kubeconfig *rest.Config) error {
	managementKubeClient, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		return err
	}

	spokeKubeClient := managementKubeClient
	if len(o.ManagedKubeconfigFile) != 0 {
		managedRestConfig, err := clientcmd.BuildConfigFromFlags("", o.ManagedKubeconfigFile)
		if err != nil {
			return err
		}
		spokeKubeClient, err = kubernetes.NewForConfig(managedRestConfig)
		if err != nil {
			return err
		}
	}

	hubRestConfig, err := clientcmd.BuildConfigFromFlags("", o.HubKubeconfigFile)
	if err != nil {
		return err
	}
	hubKubeClient, err := kubernetes.NewForConfig(hubRestConfig)
	if err != nil {
		return err
	}
	addonClient, err := addonv1alpha1client.NewForConfig(hubRestConfig)
	if err != nil {
		return err
	}
	hubClusterClient, err := clientset.NewForConfig(hubRestConfig)
	if err != nil {
		return err
	}

	hubKubeInformerFactory := informers.NewSharedInformerFactoryWithOptions(hubKubeClient, 10*time.Minute, informers.WithNamespace(o.SpokeClusterName))
	spokeInformerFactory := informers.NewSharedInformerFactoryWithOptions(spokeKubeClient, 10*time.Minute, informers.WithNamespace(common.DynamicScoringNamespace))

	_, controller := newAgentController(
		hubClusterClient,
		spokeKubeClient,
		addonClient,
		spokeInformerFactory.Core().V1().ConfigMaps(),
		o.SpokeClusterName,
		o.AddonName,
		o.AddonNamespace,
	)
	leaseUpdater := lease.NewLeaseUpdater(
		managementKubeClient,
		o.AddonName,
		o.AddonNamespace,
	)

	go hubKubeInformerFactory.Start(ctx.Done())
	go spokeInformerFactory.Start(ctx.Done())
	go controller.Run(ctx, 1)
	go leaseUpdater.Start(ctx)

	klog.Info("Started dynamic scoring agent controller.")

	go startMetricsServer()

	klog.Info("Started prometheus server for exporting dynamic score.")

	<-ctx.Done()
	return nil
}

type ScoringWorker struct {
	cancel  context.CancelFunc
	summary *common.ScorerSummary
}

type agentController struct {
	hubClusterClient          clientset.Interface
	spokeKubeClient           kubernetes.Interface
	addonClient               addonv1alpha1client.Interface
	spokeConfigMapLister      corev1lister.ConfigMapLister
	addOnPlacementScoreLister v1alpha1.AddOnPlacementScoreLister
	clusterName               string
	addonName                 string
	addonNamespace            string
	scoringWorkers            map[string]*ScoringWorker
	workerLock                sync.Mutex
	sourceAuth                map[string]string
	scoringAuth               map[string]string
	httpClient                *http.Client
	previousLabelSets         map[string]map[string]struct{}
}

func newAgentController(
	hubClusterClient clientset.Interface,
	spokeKubeClient kubernetes.Interface,
	addonClient addonv1alpha1client.Interface,
	configmapInformers corev1informers.ConfigMapInformer,
	clusterName string,
	addonName string,
	addonNamespace string,
) (*agentController, factory.Controller) {
	c := &agentController{
		hubClusterClient:     hubClusterClient,
		spokeKubeClient:      spokeKubeClient,
		addonClient:          addonClient,
		clusterName:          clusterName,
		addonName:            addonName,
		addonNamespace:       addonNamespace,
		spokeConfigMapLister: configmapInformers.Lister(),
		scoringWorkers:       make(map[string]*ScoringWorker),
		workerLock:           sync.Mutex{},
		sourceAuth:           make(map[string]string),
		scoringAuth:          make(map[string]string),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:       10,
				IdleConnTimeout:    90 * time.Second,
				DisableCompression: false,
			},
		},
		previousLabelSets: make(map[string]map[string]struct{}),
	}
	controller := factory.New().WithInformersQueueKeysFunc(
		func(obj runtime.Object) []string {
			key, _ := cache.MetaNamespaceKeyFunc(obj)
			return []string{key}
		}, configmapInformers.Informer()).
		WithSync(c.sync).ToController("dynamic-scoring-agent-controller")

	return c, controller
}

func (c *agentController) sync(ctx context.Context, syncCtx factory.SyncContext, key string) error {
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	if ns != common.DynamicScoringNamespace || name != common.DynamicScoringConfigName {
		return nil
	}

	klog.Infof("Reconciling ConfigMap: %s", key)

	cm, err := c.spokeConfigMapLister.ConfigMaps(ns).Get(name)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	c.handleConfigMapUpdate(ctx, cm)
	return nil
}

func (c *agentController) handleConfigMapUpdate(ctx context.Context, cm *corev1.ConfigMap) {
	data, ok := cm.Data["summaries"]
	if !ok {
		klog.Warning("Missing 'summaries' key in configmap")
		return
	}
	var summaries []common.ScorerSummary
	if err := json.Unmarshal([]byte(data), &summaries); err != nil {
		klog.ErrorS(err, "Invalid JSON in summaries")
		return
	}

	klog.Infof("Summay Num: %d", len(summaries))

	active := make(map[string]bool)
	for _, s := range summaries {
		active[s.ScoreName] = true
		klog.Infof("Fetch secrets : %s", s.ScoreName)
		if err := c.fetchSourceAuth(ctx, s); err != nil {
			klog.ErrorS(err, "Failed to fetch source auth, skipping scorer", "scoreName", s.ScoreName)
			delete(active, s.ScoreName)
			continue
		}
		if err := c.fetchScoringAuth(ctx, s); err != nil {
			klog.ErrorS(err, "Failed to fetch scoring auth, skipping scorer", "scoreName", s.ScoreName)
			delete(active, s.ScoreName)
			continue
		}
		klog.Infof("Start target : %s", s.ScoreName)
		c.workerLock.Lock()
		if _, exists := c.scoringWorkers[s.ScoreName]; !exists {
			ctxWorker, cancel := context.WithCancel(ctx)
			c.scoringWorkers[s.ScoreName] = &ScoringWorker{
				cancel:  cancel,
				summary: &s,
			}
			c.workerLock.Unlock()
			go c.startScoringLoop(ctxWorker, c.scoringWorkers[s.ScoreName].summary)
			klog.Infof("Started scoring worker for %s", s.ScoreName)
		} else {
			c.scoringWorkers[s.ScoreName].summary = &s
			c.workerLock.Unlock()
			klog.Infof("Scoring worker already exists for %s", s.ScoreName)
		}
	}

	var workersToStop []string

	c.workerLock.Lock()
	for name, worker := range c.scoringWorkers {
		if !active[name] {
			worker.cancel()
			workersToStop = append(workersToStop, name)
			delete(c.scoringWorkers, name)
			klog.Infof("Stopped scoring worker for %s", name)
		}
	}
	c.workerLock.Unlock()

	// Delete metrics for stopped workers
	for _, name := range workersToStop {
		dynamicScoreGauge.DeletePartialMatch(prometheus.Labels{
			common.DynamicScoreLabelPrefix + "score_name": name,
		})
		klog.Infof("Deleted metrics for stopped worker %s", name)
	}
}

func (c *agentController) startScoringLoop(ctx context.Context, summary *common.ScorerSummary) {
	ticker := time.NewTicker(time.Duration(summary.ScoringInterval) * time.Second)
	defer ticker.Stop()

	klog.Infof("Start Scoring Loop : %s", summary.ScoreName)

	for {
		select {
		case <-ctx.Done():
			klog.Infof("Exiting scoring loop for %s", summary.ScoreName)
			return
		case <-ticker.C:
			if err := c.performScoring(ctx, summary); err != nil {
				klog.Errorf("Scoring error (%s): %v", summary.ScoreName, err)
			}
		}
	}
}

type ScoringRequest struct {
	Data interface{} `json:"data"`
}

type ScoringResult struct {
	Metric map[string]string `json:"metric"`
	Value  float64           `json:"score"`
}

type ScoringResponse struct {
	Results []ScoringResult `json:"results"`
}

func (c *agentController) performScoring(ctx context.Context, summary *common.ScorerSummary) error {
	klog.Infof("performScoring start : %s", summary.ScoreName)

	var scoringInputData []struct {
		Metric map[string]string `json:"metric"`
		Values [][]interface{}   `json:"values"`
	}

	// Fetch source data
	if summary.SourceType == "prometheus" {
		parsedQueries := strings.Split(summary.SourceQuery, ";")
		klog.Infof("Parsed Queries Count: %d for %s", len(parsedQueries), summary.ScoreName)

		end := time.Now()
		start := end.Add(-time.Duration(summary.SourceRange) * time.Second)

		for _, q := range parsedQueries {
			params := url.Values{
				"query": []string{q},
				"start": []string{fmt.Sprintf("%d", start.Unix())},
				"end":   []string{fmt.Sprintf("%d", end.Unix())},
				"step":  []string{fmt.Sprintf("%d", summary.SourceStep)},
			}
			fullURL := fmt.Sprintf("%s?%s", summary.SourceEndpoint, params.Encode())

			req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
			if err != nil {
				return fmt.Errorf("failed to create prometheus query request (%s): %w", summary.ScoreName, err)
			}
			// sourceHeader
			if token, ok := c.sourceAuth[summary.ScoreName]; ok && token != "" {
				req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
			}

			resp, err := c.httpClient.Do(req)
			if err != nil {
				return fmt.Errorf("failed to query prometheus (%s): %w", summary.ScoreName, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("prometheus query failed with status %d for %s", resp.StatusCode, summary.ScoreName)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("failed to read prometheus response: %w", err)
			}

			var promResp struct {
				Status string `json:"status"`
				Data   struct {
					Result []struct {
						Metric map[string]string `json:"metric"`
						Values [][]interface{}   `json:"values"`
					} `json:"result"`
				} `json:"data"`
			}
			if err := json.Unmarshal(body, &promResp); err != nil {
				return fmt.Errorf("Prometheus Response JSON unmarshal error: %w", err)
			}
			if promResp.Status != "success" {
				return fmt.Errorf("Prometheus query failed: %s", promResp.Status)
			}
			scoringInputData = append(scoringInputData, promResp.Data.Result...)
		}
	}

	scoringPayload := map[string]interface{}{"data": scoringInputData}
	payloadBytes, err := json.Marshal(scoringPayload)

	if err != nil {
		return fmt.Errorf("failed to marshal scoring payload: %w", err)
	}

	req2, err := http.NewRequestWithContext(ctx, "POST", summary.ScoringEndpoint, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create scoring request (%s): %w", summary.ScoreName, err)
	}

	// scoringHeader
	req2.Header.Set("Content-Type", "application/json")
	if token, ok := c.scoringAuth[summary.ScoreName]; ok && token != "" {
		req2.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	}

	resp2, err := c.httpClient.Do(req2)
	if err != nil {
		return fmt.Errorf("failed to query scoring endpoint (%s): %w", summary.ScoreName, err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		return fmt.Errorf("scoring endpoint returned status %d for %s", resp2.StatusCode, summary.ScoreName)
	}

	resp2Body, err := io.ReadAll(resp2.Body)
	if err != nil {
		return fmt.Errorf("failed to read scoring response (%s): %w", summary.ScoreName, err)
	}

	var scoringResp struct {
		Results []struct {
			Metric map[string]string `json:"metric"`
			Value  float64           `json:"score"`
		} `json:"results"`
	}
	if err := json.Unmarshal(resp2Body, &scoringResp); err != nil {
		return fmt.Errorf("Scoring Response JSON unmarshal error: %w", err)
	}

	currentLabels := make(map[string]struct{})
	currentPlaceHolderMapping := make(map[string]map[string]string)
	currentScores := make(map[string]float64)

	for _, r := range scoringResp.Results {
		labels := []string{
			c.clusterName,
			getValidString(r.Metric, "node", true),
			getValidString(r.Metric, "device", true),
			getValidString(r.Metric, "namespace", true),
			getValidString(r.Metric, "app", true),
			getValidString(r.Metric, "pod", true),
			getValidString(r.Metric, "container", true),
			getValidString(r.Metric, "meta", true),
			summary.ScoreName,
		}
		placeHolderMapping := map[string]string{
			"${cluster}":   c.clusterName,
			"${node}":      getValidString(r.Metric, "node", false),
			"${device}":    getValidString(r.Metric, "device", false),
			"${namespace}": getValidString(r.Metric, "namespace", false),
			"${app}":       getValidString(r.Metric, "app", false),
			"${pod}":       getValidString(r.Metric, "pod", false),
			"${container}": getValidString(r.Metric, "container", false),
			"${meta}":      getValidString(r.Metric, "meta", false),
			"${scoreName}": summary.ScoreName,
		}
		aggregatedLabels := aggregateLabels(labels)
		currentPlaceHolderMapping[aggregatedLabels] = placeHolderMapping
		currentScores[aggregatedLabels] = r.Value
		dynamicScoreGauge.WithLabelValues(labels...).Set(r.Value)

		currentLabels[labelKey(labels...)] = struct{}{}
	}

	klog.Infof("performScoring success : %s", summary.ScoreName)

	// 前回との比較と削除
	if prev, ok := c.previousLabelSets[summary.ScoreName]; ok {
		for labelStr := range prev {
			if _, stillExists := currentLabels[labelStr]; !stillExists {
				labels := strings.Split(labelStr, "||")
				dynamicScoreGauge.DeleteLabelValues(labels...)
			}
		}
	}

	c.previousLabelSets[summary.ScoreName] = currentLabels

	if summary.ScoreDestination == "AddOnPlacementScore" {
		if err := c.updateAddonPlacementScore(ctx, summary, currentPlaceHolderMapping, currentScores); err != nil {
			return fmt.Errorf("failed to update AddOnPlacementScore (%s): %w", summary.ScoreName, err)
		}
	}

	return nil
}

func getValidString(m map[string]string, key string, truncate bool) string {
	if val, ok := m[key]; ok {
		if truncate && len(val) > common.DynamicScoreLabelMaxLength {
			val = val[:common.DynamicScoreLabelMaxLength]
		}
		return val
	}
	return ""
}

func (c *agentController) fetchSourceAuth(ctx context.Context, summary common.ScorerSummary) error {
	if summary.SourceEndpointAuthName != "" && summary.SourceEndpointAuthKey != "" {
		secret, err := c.spokeKubeClient.CoreV1().
			Secrets(c.addonNamespace).
			Get(ctx, summary.SourceEndpointAuthName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get source auth secret (%s): %w", summary.ScoreName, err)
		} else {
			if val, ok := secret.Data[summary.SourceEndpointAuthKey]; ok {
				c.sourceAuth[summary.ScoreName] = string(val)
			} else {
				return fmt.Errorf("source auth key not found in secret (%s): %s", summary.ScoreName, summary.SourceEndpointAuthKey)
			}
		}
	}
	return nil
}

func (c *agentController) fetchScoringAuth(ctx context.Context, summary common.ScorerSummary) error {
	if summary.ScoringEndpointAuthName != "" && summary.ScoringEndpointAuthKey != "" {
		secret, err := c.spokeKubeClient.CoreV1().
			Secrets(c.addonNamespace).
			Get(ctx, summary.ScoringEndpointAuthName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get scoring auth secret (%s): %w", summary.ScoreName, err)
		} else {
			if val, ok := secret.Data[summary.ScoringEndpointAuthKey]; ok {
				c.scoringAuth[summary.ScoreName] = string(val)
			} else {
				return fmt.Errorf("scoring auth key not found in secret (%s): %s", summary.ScoreName, summary.ScoringEndpointAuthKey)
			}
		}
	}
	return nil
}

func (c *agentController) updateAddonPlacementScore(ctx context.Context, summary *common.ScorerSummary, currentPlaceHolderMapping map[string]map[string]string, currentScores map[string]float64) error {

	// check https://github.com/open-cluster-management-io/addon-contrib/blob/main/resource-usage-collect-addon/pkg/addon/agent/agent.go
	sanitizedName := sanitizeResourceName(summary.ScoreName)
	klog.Info("Checking AddOnPlacementScore: ", c.clusterName, sanitizedName)

	addonPlacementScore, err := c.hubClusterClient.ClusterV1alpha1().AddOnPlacementScores(c.clusterName).Get(ctx, sanitizedName, metav1.GetOptions{})
	klog.Info("Current AddOnPlacementScore: ", addonPlacementScore, "err: ", err)

	items := []apiv1alpha2.AddOnPlacementScoreItem{}
	dimensionValueMap := make(map[string]int32)

	for labelStr, score := range currentScores {
		renderedDimentionName := renderDimansionName(summary.ScoreDimensionFormat, currentPlaceHolderMapping[labelStr])
		if renderedDimentionName == "" {
			continue
		}
		dimensionValueMap[renderedDimentionName] = int32(score)
	}
	for dimension, value := range dimensionValueMap {
		items = append(items, apiv1alpha2.AddOnPlacementScoreItem{
			Name:  dimension,
			Value: value,
		})
	}

	switch {
	case errors.IsNotFound(err):
		klog.Info("not found AddOnPlacementScore, creating new one")
		klog.Info(err)
		addonPlacementScore = &apiv1alpha2.AddOnPlacementScore{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: c.clusterName,
				Name:      sanitizedName,
			},
			Status: apiv1alpha2.AddOnPlacementScoreStatus{
				Scores: items,
			},
		}
		_, err = c.hubClusterClient.ClusterV1alpha1().AddOnPlacementScores(c.clusterName).Create(ctx, addonPlacementScore, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		return nil
	case err != nil:
		return err
	}

	klog.Info("AddOnPlacementScore found, updating existing one")
	addonPlacementScore.Status.Scores = items
	_, err = c.hubClusterClient.ClusterV1alpha1().AddOnPlacementScores(c.clusterName).UpdateStatus(ctx, addonPlacementScore, metav1.UpdateOptions{})
	return err
}

func labelKey(values ...string) string {
	return strings.Join(values, "||") // シンプルなハッシュ
}

func sanitizeResourceName(name string) string {
	if name == "" {
		return "score"
	}
	// lowercase
	s := strings.ToLower(name)

	// replace any char not in [a-z0-9.-] with '-'
	reInvalid := regexp.MustCompile(`[^a-z0-9\.-]`)
	s = reInvalid.ReplaceAllString(s, "-")

	// collapse multiple '-'
	reDash := regexp.MustCompile(`-+`)
	s = reDash.ReplaceAllString(s, "-")

	// trim leading/trailing '-' or '.'
	s = strings.Trim(s, "-.")
	// ensure length <= 253
	if len(s) > 253 {
		s = s[:253]
		// if cut ends with non-alnum, trim trailing non-alnum
		s = strings.TrimRightFunc(s, func(r rune) bool {
			return !(('a' <= r && r <= 'z') || ('0' <= r && r <= '9'))
		})
	}

	// ensure starts with alnum
	if s == "" {
		s = "score"
	}
	first := s[0]
	last := s[len(s)-1]
	if !(('a' <= first && first <= 'z') || ('0' <= first && first <= '9')) {
		s = "a" + s
	}
	if !(('a' <= last && last <= 'z') || ('0' <= last && last <= '9')) {
		s = s + "a"
	}

	return s
}

func aggregateLabels(labels []string) string {
	noneEmptyLabels := []string{}
	for _, label := range labels {
		if label != "" {
			noneEmptyLabels = append(noneEmptyLabels, label)
		}
	}
	return strings.Join(noneEmptyLabels, ";")
}

func renderDimansionName(format string, placeHolderMapping map[string]string) string {
	result := format
	for placeholder, value := range placeHolderMapping {
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}
