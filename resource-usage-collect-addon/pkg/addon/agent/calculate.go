package agent

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	corev1informers "k8s.io/client-go/informers/core/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

// MAXSCORE Constants for scores
const MAXSCORE = float64(100)
const MINSCORE = float64(-100)

// MAXCPUCOUNT Constants for CPU resource counts
const MAXCPUCOUNT = float64(100)
const MINCPUCOUNT = float64(0)

// MAXGPUCOUNT Constants for GPU resource counts
const MAXGPUCOUNT = float64(20) // Assume that one cluster can have maximum 10 GPUs, can be modified.
const MINGPUCOUNT = float64(0)

// MAXTPUCOUNT Constants for TPU resource counts
const MAXTPUCOUNT = float64(20) // Assume that one cluster can have maximum 10 GPUs, can be modified.
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

func (s *Score) calculateScore() (cpuScore int64, memScore int64, gpuScore int64, tpuScore int64, err error) {
	cpuAlloc, err := s.calculateClusterAllocatable(string(clusterv1.ResourceCPU))
	if err != nil {
		return 0, 0, 0, 0, err
	}
	memAlloc, err := s.calculateClusterAllocatable(string(clusterv1.ResourceMemory))
	if err != nil {
		return 0, 0, 0, 0, err
	}
	gpuAlloc, err := s.calculateClusterAllocatable(ResourceGPU)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	tpuAlloc, err := s.calculateClusterAllocatable(ResourceTPU)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	cpuUsage, err := s.calculatePodResourceRequest(string(v1.ResourceCPU))
	if err != nil {
		return 0, 0, 0, 0, err
	}
	memUsage, err := s.calculatePodResourceRequest(string(v1.ResourceMemory))
	if err != nil {
		return 0, 0, 0, 0, err
	}
	gpuUsage, err := s.calculatePodResourceRequest(ResourceGPU)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	tpuUsage, err := s.calculatePodResourceRequest(ResourceTPU)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	return s.normalizeScore(cpuAlloc, cpuUsage, memAlloc, memUsage, gpuAlloc, gpuUsage, tpuAlloc, tpuUsage)
}

func (s *Score) normalizeScore(cpuAlloc, cpuUsage, memAlloc, memUsage, gpuAlloc, gpuUsage, tpuAlloc, tpuUsage float64) (cpuScore int64, memScore int64, gpuScore int64, tpuScore int64, err error) {
	klog.Infof("cpuAlloc = %v, cpuUsage = %v, memAlloc = %v, memUsage = %v, gpuAlloc = %v, gpuUsage = %v, tpuAlloc = %v, tpuUsage = %v", cpuAlloc, cpuUsage, memAlloc, memUsage, gpuAlloc, gpuUsage, tpuAlloc, tpuUsage)
	availableCpu := cpuAlloc - cpuUsage
	if availableCpu > MAXCPUCOUNT {
		cpuScore = int64(MAXSCORE)
	} else if availableCpu <= MINCPUCOUNT {
		cpuScore = int64(MINSCORE)
	} else {
		cpuScore = int64(200*availableCpu/MAXCPUCOUNT - 100)
	}

	availableMem := (memAlloc - memUsage) / (1024 * 1024) // MB
	if availableMem > MAXMEMCOUNT {
		memScore = int64(MAXSCORE)
	} else if availableMem <= MINMEMCOUNT {
		memScore = int64(MINSCORE)
	} else {
		memScore = int64(200*availableMem/MAXMEMCOUNT - 100)
	}

	availableGpu := gpuAlloc - gpuUsage
	if availableGpu > MAXGPUCOUNT {
		gpuScore = int64(MAXSCORE)
	} else if availableGpu <= MINGPUCOUNT {
		gpuScore = int64(MINSCORE)
	} else {
		gpuScore = int64(200*availableGpu/MAXGPUCOUNT - 100)
	}

	availableTpu := tpuAlloc - tpuUsage
	if availableTpu > MAXTPUCOUNT {
		tpuScore = int64(MAXSCORE)
	} else if availableTpu <= MINTPUCOUNT {
		tpuScore = int64(MINSCORE)
	} else {
		tpuScore = int64(200*availableTpu/MAXTPUCOUNT - 100)
	}

	klog.Infof("cpuScore = %v, memScore = %v, gpuScore = %v, tpuScore = %v", cpuScore, memScore, gpuScore, tpuScore)
	return cpuScore, memScore, gpuScore, tpuScore, nil
}

func (s *Score) calculateClusterAllocatable(resourceName string) (float64, error) {
	nodes, err := s.nodeLister.List(labels.Everything())
	if err != nil {
		return 0, err
	}

	allocatableList := make(map[string]resource.Quantity)
	for _, node := range nodes {
		if node.Spec.Unschedulable {
			continue
		}
		for key, value := range node.Status.Allocatable {
			if allocatable, exist := allocatableList[string(key)]; exist {
				allocatable.Add(value)
				allocatableList[string(key)] = allocatable
			} else {
				allocatableList[string(key)] = value
			}
		}
	}
	quantity, exists := allocatableList[resourceName]
	if !exists {
		return 0, nil
	}
	return quantity.AsApproximateFloat64(), nil
}

func (s *Score) calculatePodResourceRequest(resourceName string) (float64, error) {
	list, err := s.podLister.List(labels.Everything())
	if err != nil {
		return 0, err
	}

	var podRequest float64
	for _, pod := range list {
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
