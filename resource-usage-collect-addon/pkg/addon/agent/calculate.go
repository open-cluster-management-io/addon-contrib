package agent

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	corev1informers "k8s.io/client-go/informers/core/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clustersdkv1alpha1 "open-cluster-management.io/sdk-go/pkg/apis/cluster/v1alpha1"
)

const MAXCPUCOUNT = float64(100)
const MINCPUCOUNT = float64(0)

// // 1TB
const MAXMEMCOUNT = float64(1024 * 1024)
const MINMEMCOUNT = float64(0)

type Score struct {
	nodeLister        corev1lister.NodeLister
	useRequested      bool
	enablePodOverhead bool
	podListener       corev1lister.PodLister
}

func NewScore(nodeInformer corev1informers.NodeInformer, podInformer corev1informers.PodInformer) *Score {
	return &Score{
		nodeLister:        nodeInformer.Lister(),
		podListener:       podInformer.Lister(),
		enablePodOverhead: true,
		useRequested:      true,
	}
}

func (s *Score) calculateScore() (cpuScore int32, memScore int32, err error) {
	cpuAlloc, err := s.calculateClusterAllocateable(clusterv1.ResourceCPU)
	if err != nil {
		return 0, 0, err
	}
	memAlloc, err := s.calculateClusterAllocateable(clusterv1.ResourceMemory)
	if err != nil {
		return 0, 0, err
	}

	cpuUsage, err := s.calculatePodResourceRequest(v1.ResourceCPU)
	if err != nil {
		return 0, 0, err
	}
	memUsage, err := s.calculatePodResourceRequest(v1.ResourceMemory)
	if err != nil {
		return 0, 0, err
	}

	return s.normalizeScore(cpuAlloc, cpuUsage, memAlloc, memUsage)
}

func (s *Score) normalizeScore(cpuAlloc, cpuUsage, memAlloc, memUsage float64) (cpuScore int32, memScore int32, err error) {
	vailableCpu := cpuAlloc - cpuUsage
	availableMem := (memAlloc - memUsage) / (1024 * 1024) // MB

	cpuScoreNormalizer := clustersdkv1alpha1.NewScoreNormalizer(MINCPUCOUNT, MAXCPUCOUNT)
	cpuScore, err = cpuScoreNormalizer.Normalize(vailableCpu)

	memScoreNormalizer := clustersdkv1alpha1.NewScoreNormalizer(MINMEMCOUNT, MAXMEMCOUNT)
	memScore, err = memScoreNormalizer.Normalize(availableMem)

	return cpuScore, memScore, err
}

func (s *Score) calculateClusterAllocateable(resourceName clusterv1.ResourceName) (float64, error) {
	nodes, err := s.nodeLister.List(labels.Everything())
	if err != nil {
		return 0, err
	}

	allocatableList := make(map[clusterv1.ResourceName]resource.Quantity)
	for _, node := range nodes {
		if node.Spec.Unschedulable {
			continue
		}
		for key, value := range node.Status.Allocatable {
			if allocatable, exist := allocatableList[clusterv1.ResourceName(key)]; exist {
				allocatable.Add(value)
				allocatableList[clusterv1.ResourceName(key)] = allocatable
			} else {
				allocatableList[clusterv1.ResourceName(key)] = value
			}
		}
	}
	quantity := allocatableList[resourceName]
	return quantity.AsApproximateFloat64(), nil
}

func (s *Score) calculatePodResourceRequest(resourceName v1.ResourceName) (float64, error) {
	list, err := s.podListener.List(labels.Everything())
	if err != nil {
		return 0, err
	}

	var podRequest float64
	var podCount int
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
			if quantity, found := pod.Spec.Overhead[resourceName]; found {
				podRequest += quantity.AsApproximateFloat64()
			}
		}
		podCount++
	}
	return podRequest, nil
}

func (s *Score) getRequestForResource(resource v1.ResourceName, requests *v1.ResourceList, nonZero bool) float64 {
	if requests == nil {
		return 0
	}
	switch resource {
	case v1.ResourceCPU:
		// Override if un-set, but not if explicitly set to zero
		if _, found := (*requests)[v1.ResourceCPU]; !found && nonZero {
			return 100
		}
		return requests.Cpu().AsApproximateFloat64()
	case v1.ResourceMemory:
		// Override if un-set, but not if explicitly set to zero
		if _, found := (*requests)[v1.ResourceMemory]; !found && nonZero {
			return 200 * 1024 * 1024
		}
		return requests.Memory().AsApproximateFloat64()
	default:
		quantity, found := (*requests)[resource]
		if !found {
			return 0
		}
		return quantity.AsApproximateFloat64()
	}
}
