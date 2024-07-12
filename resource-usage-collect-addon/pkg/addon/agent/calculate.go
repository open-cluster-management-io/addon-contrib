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

func (s *Score) calculateScore() (cpuScore int32, memScore int32, gpuScore int32, tpuScore int32, err error) {
	cpuAlloc, cpuNode, err := s.calculateClusterAllocatable(string(clusterv1.ResourceCPU))
	if err != nil {
		return 0, 0, 0, 0, err
	}
	memAlloc, memNode, err := s.calculateClusterAllocatable(string(clusterv1.ResourceMemory))
	if err != nil {
		return 0, 0, 0, 0, err
	}
	gpuAlloc, gpuNode, err := s.calculateClusterAllocatable(ResourceGPU)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	tpuAlloc, tpuNode, err := s.calculateClusterAllocatable(ResourceTPU)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	cpuUsage, err := s.calculateNodeResourceUsage(cpuNode, string(v1.ResourceCPU))
	if err != nil {
		return 0, 0, 0, 0, err
	}
	memUsage, err := s.calculateNodeResourceUsage(memNode, string(v1.ResourceMemory))
	if err != nil {
		return 0, 0, 0, 0, err
	}
	gpuUsage, err := s.calculateNodeResourceUsage(gpuNode, ResourceGPU)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	tpuUsage, err := s.calculateNodeResourceUsage(tpuNode, ResourceTPU)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	return s.normalizeScore(cpuAlloc, cpuUsage, memAlloc, memUsage, gpuAlloc, gpuUsage, tpuAlloc, tpuUsage)
}

func (s *Score) normalizeScore(cpuAlloc, cpuUsage, memAlloc, memUsage, gpuAlloc, gpuUsage, tpuAlloc, tpuUsage float64) (cpuScore int32, memScore int32, gpuScore int32, tpuScore int32, err error) {
	klog.Infof("cpuAlloc = %v, cpuUsage = %v, memAlloc = %v, memUsage = %v, gpuAlloc = %v, gpuUsage = %v, tpuAlloc = %v, tpuUsage = %v", cpuAlloc, cpuUsage, memAlloc, memUsage, gpuAlloc, gpuUsage, tpuAlloc, tpuUsage)
	availableCpu := cpuAlloc - cpuUsage
	cpuScoreNormalizer := clustersdkv1alpha1.NewScoreNormalizer(MINCPUCOUNT, MAXCPUCOUNT)
	cpuScore, err = cpuScoreNormalizer.Normalize(availableCpu)

	availableMem := (memAlloc - memUsage) / (1024 * 1024) // MB
	memScoreNormalizer := clustersdkv1alpha1.NewScoreNormalizer(MINMEMCOUNT, MAXMEMCOUNT)
	memScore, err = memScoreNormalizer.Normalize(availableMem)

	availableGpu := gpuAlloc - gpuUsage
	gpuScoreNormalizer := clustersdkv1alpha1.NewScoreNormalizer(MINGPUCOUNT, MAXGPUCOUNT)
	gpuScore, err = gpuScoreNormalizer.Normalize(availableGpu)

	availableTpu := tpuAlloc - tpuUsage
	tpuScoreNormalizer := clustersdkv1alpha1.NewScoreNormalizer(MINTPUCOUNT, MAXTPUCOUNT)
	tpuScore, err = tpuScoreNormalizer.Normalize(availableTpu)

	klog.Infof("cpuScore = %v, memScore = %v, gpuScore = %v, tpuScore = %v", cpuScore, memScore, gpuScore, tpuScore)
	return cpuScore, memScore, gpuScore, tpuScore, nil
}

// Iterate every node, find the node with maximum allocatable resource, return the number and node name.
func (s *Score) calculateClusterAllocatable(resourceName string) (float64, string, error) {
	nodes, err := s.nodeLister.List(labels.Everything())
	if err != nil {
		return 0, "", err
	}

	var maxAllocatable float64
	var maxNodeName string
	for _, node := range nodes {
		if node.Spec.Unschedulable {
			continue
		}
		alloc, exists := node.Status.Allocatable[v1.ResourceName(resourceName)]
		if !exists {
			continue
		}
		klog.Infof("Node: %s, Allocatable %s: %f", node.Name, resourceName, alloc.AsApproximateFloat64())
		if alloc.AsApproximateFloat64() > maxAllocatable {
			maxAllocatable = alloc.AsApproximateFloat64()
			maxNodeName = node.Name
		}
	}
	klog.Infof("Max allocatable %s: %f on node: %s", resourceName, maxAllocatable, maxNodeName)
	return maxAllocatable, maxNodeName, nil
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

func (s *Score) calculateNodeResourceUsage(nodeName string, resourceName string) (float64, error) {
	list, err := s.podLister.List(labels.Everything())
	if err != nil {
		return 0, err
	}

	var podRequest float64
	for _, pod := range list {
		if pod.Spec.NodeName != nodeName {
			continue
		}
		for i := range pod.Spec.Containers {
			container := &pod.Spec.Containers[i]
			value := s.getRequestForResource(resourceName, &container.Resources.Requests, !s.useRequested)
			podRequest += value
		}

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
