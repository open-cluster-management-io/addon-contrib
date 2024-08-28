package agent

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	corev1informers "k8s.io/client-go/informers/core/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clustersdkv1alpha1 "open-cluster-management.io/sdk-go/pkg/apis/cluster/v1alpha1"
)

// MAXCPUCOUNT Constants for CPU resource counts
const MAXCPUCOUNT = float64(100)
const MINCPUCOUNT = float64(0)

// MAXGPUCOUNT Constants for GPU resource counts
const MAXGPUCOUNT = float64(20) // Assume that one cluster can have maximum 20 GPUs, can be modified.
const MINGPUCOUNT = float64(0)

// MAXTPUCOUNT Constants for TPU resource counts
const MAXTPUCOUNT = float64(20) // Assume that one cluster can have maximum 20 TPUs, can be modified.
const MINTPUCOUNT = float64(0)

// MAXMEMCOUNT Constants for memory
const MAXMEMCOUNT = float64(1024 * 1024)
const MINMEMCOUNT = float64(0)

// ResourceGPU Custom resource names
const ResourceGPU = "nvidia.com/gpu"
const ResourceTPU = "google.com/tpu"

type Score struct {
	nodeLister        corev1lister.NodeLister
	useRequested      bool
	enablePodOverhead bool
	podLister         corev1lister.PodLister
}

func NewScore(nodeInformer corev1informers.NodeInformer, podInformer corev1informers.PodInformer) *Score {
	return &Score{
		nodeLister:        nodeInformer.Lister(),
		podLister:         podInformer.Lister(),
		enablePodOverhead: true,
		useRequested:      true,
	}
}

// Calculate the available resources in the node scope, the node with the maximum available resources will be chosen to calculate the score.
func (s *Score) calculateNodeScore() (cpuScore int32, memScore int32, gpuScore int32, tpuScore int32, err error) {
	// Get the amount of resources available for the node with the largest actual available CPU resources.
	cpuAvailable, _, err := s.calculateMaxAvailableNode(string(clusterv1.ResourceCPU))
	if err != nil {
		return 0, 0, 0, 0, err
	}
	// Get the amount of resources available for the node with the largest actual available Memory resources.
	memAvailable, _, err := s.calculateMaxAvailableNode(string(clusterv1.ResourceMemory))
	if err != nil {
		return 0, 0, 0, 0, err
	}
	// Get the amount of resources available for the node with the largest actual available GPU resources.
	gpuAvailable, _, err := s.calculateMaxAvailableNode(ResourceGPU)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	// Get the amount of resources available for the node with the largest actual available TPU resources.
	tpuAvailable, _, err := s.calculateMaxAvailableNode(ResourceTPU)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	// Use the amount of available resources directly to generate scores
	return s.normalizeScore("node", cpuAvailable, memAvailable, gpuAvailable, tpuAvailable)
}

// Calculate the available resources in the cluster scope and return four scores for CPU, Memory, GPU, and TPU.
func (s *Score) calculateClusterScopeScore() (cpuScore int32, memScore int32, gpuScore int32, tpuScore int32, err error) {
	// Get the total available CPU resources across the cluster.
	cpuAvailable, err := s.calculateClusterAvailable(string(clusterv1.ResourceCPU))
	if err != nil {
		return 0, 0, 0, 0, err
	}

	// Get the total available Memory resources across the cluster.
	memAvailable, err := s.calculateClusterAvailable(string(clusterv1.ResourceMemory))
	if err != nil {
		return 0, 0, 0, 0, err
	}

	// Get the total available GPU resources across the cluster.
	gpuAvailable, err := s.calculateClusterAvailable(ResourceGPU)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	// Get the total available TPU resources across the cluster.
	tpuAvailable, err := s.calculateClusterAvailable(ResourceTPU)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	// Normalize and return the scores based on available resources
	return s.normalizeScore("cluster", cpuAvailable, memAvailable, gpuAvailable, tpuAvailable)
}

// Calculate the available resources in the cluster scope.
func (s *Score) calculateClusterAvailable(resourceName string) (float64, error) {
	nodes, err := s.nodeLister.List(labels.Everything())
	if err != nil {
		return 0, err
	}

	var totalAllocatable float64
	var totalUsage float64

	for _, node := range nodes {
		if node.Spec.Unschedulable {
			continue
		}

		// Accumulate allocatable resources from all nodes
		alloc, exists := node.Status.Allocatable[v1.ResourceName(resourceName)]
		if exists {
			totalAllocatable += alloc.AsApproximateFloat64()
		}

		// Calculate the resource usage for this node
		usage, err := s.calculateNodeResourceUsage(node.Name, resourceName)
		if err != nil {
			return 0, err
		}
		totalUsage += usage
	}

	// Calculate available resources
	available := totalAllocatable - totalUsage
	return available, nil
}

// Normalize the score with the logic of ScoreNormaliser.
func (s *Score) normalizeScore(scope string, cpuAvailable, memAvailable, gpuAvailable, tpuAvailable float64) (cpuScore int32, memScore int32, gpuScore int32, tpuScore int32, err error) {
	// Add a parameter that identifies whether the current scope is "cluster scope" or "node scope".
	klog.Infof("[%s] cpuAvailable = %v, memAvailable = %v, gpuAvailable = %v, tpuAvailable = %v", scope, cpuAvailable, memAvailable, gpuAvailable, tpuAvailable)

	cpuScoreNormalizer := clustersdkv1alpha1.NewScoreNormalizer(MINCPUCOUNT, MAXCPUCOUNT)
	cpuScore, err = cpuScoreNormalizer.Normalize(cpuAvailable)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	availableMem := memAvailable / 1024 * 1024 // MB
	memScoreNormalizer := clustersdkv1alpha1.NewScoreNormalizer(MINMEMCOUNT, MAXMEMCOUNT)
	memScore, err = memScoreNormalizer.Normalize(availableMem)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	gpuScoreNormalizer := clustersdkv1alpha1.NewScoreNormalizer(MINGPUCOUNT, MAXGPUCOUNT)
	gpuScore, err = gpuScoreNormalizer.Normalize(gpuAvailable)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	tpuScoreNormalizer := clustersdkv1alpha1.NewScoreNormalizer(MINTPUCOUNT, MAXTPUCOUNT)
	tpuScore, err = tpuScoreNormalizer.Normalize(tpuAvailable)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	klog.Infof("[%s] cpuScore = %v, memScore = %v, gpuScore = %v, tpuScore = %v", scope, cpuScore, memScore, gpuScore, tpuScore)
	return cpuScore, memScore, gpuScore, tpuScore, nil
}

// Find the node in the cluster that has the maximum available resources.
func (s *Score) calculateMaxAvailableNode(resourceName string) (float64, string, error) {
	// Get the list of all Nodes,
	nodes, err := s.nodeLister.List(labels.Everything())
	if err != nil {
		return 0, "", err
	}
	var maxAvailable float64
	var maxNodeName string
	// Iterate every node, calculate its available resources amount.
	for _, node := range nodes {
		if node.Spec.Unschedulable {
			continue
		}
		alloc, exists := node.Status.Allocatable[v1.ResourceName(resourceName)]
		if !exists {
			continue
		}
		// Get the resource usage on this node.
		usage, err := s.calculateNodeResourceUsage(node.Name, resourceName)
		if err != nil {
			return 0, "", err
		}
		// Calculate the actual amount of resources available.
		available := alloc.AsApproximateFloat64() - usage
		// Find the node with the maximum available resources.
		if available > maxAvailable {
			maxAvailable = available
			maxNodeName = node.Name
		}
	}
	klog.Infof("Max available %s: %f on node: %s", resourceName, maxAvailable, maxNodeName)
	return maxAvailable, maxNodeName, nil
}

// Calculate the actual usage of a specific resource (e.g., GPU) by unfinished Pods on a given node.
func (s *Score) calculateNodeResourceUsage(nodeName string, resourceName string) (float64, error) {
	// Get the list of all Pods.
	list, err := s.podLister.List(labels.Everything())
	if err != nil {
		return 0, err
	}

	var podRequest float64
	for _, pod := range list {
		// Only counts Pods dispatched to specific nodes.
		if pod.Spec.NodeName != nodeName {
			continue
		}

		// Skip completed Pods or Pods that have released resources.
		if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed || pod.DeletionTimestamp != nil {
			continue
		}

		// Calculate resource requests for each container in the Pod.
		for i := range pod.Spec.Containers {
			container := &pod.Spec.Containers[i]
			value := s.getRequestForResource(resourceName, &container.Resources.Requests, !s.useRequested)
			podRequest += value
		}

		// Calculate resource requests for the Init container.
		for i := range pod.Spec.InitContainers {
			initContainer := &pod.Spec.InitContainers[i]
			value := s.getRequestForResource(resourceName, &initContainer.Resources.Requests, !s.useRequested)
			if podRequest < value {
				podRequest = value
			}
		}

		// If Overhead is being utilized, add to the total requests for the pod
		if pod.Spec.Overhead != nil && s.enablePodOverhead {
			if quantity, found := pod.Spec.Overhead[v1.ResourceName(resourceName)]; found {
				podRequest += quantity.AsApproximateFloat64()
			}
		}
	}
	return podRequest, nil
}

func (s *Score) getRequestForResource(resource string, requests *v1.ResourceList, nonZero bool) float64 {
	if requests == nil {
		return 0
	}
	switch resource {
	case string(v1.ResourceCPU):
		// Override if un-set, but not if explicitly set to zero
		if _, found := (*requests)[v1.ResourceCPU]; !found && nonZero {
			return 100
		}
		return requests.Cpu().AsApproximateFloat64()
	case string(v1.ResourceMemory):
		// Override if un-set, but not if explicitly set to zero
		if _, found := (*requests)[v1.ResourceMemory]; !found && nonZero {
			return 200 * 1024 * 1024
		}
		return requests.Memory().AsApproximateFloat64()
	default:
		quantity, found := (*requests)[v1.ResourceName(resource)]
		if !found {
			return 0
		}
		return quantity.AsApproximateFloat64()
	}
}
