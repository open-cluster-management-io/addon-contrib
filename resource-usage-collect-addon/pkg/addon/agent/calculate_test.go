package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

// Test normalize Score function.
func TestNormalizeValue(t *testing.T) {
	cases := []struct {
		name           string
		cpuAlloc       float64
		cpuUsage       float64
		memAlloc       float64
		memUsage       float64
		gpuAlloc       float64
		gpuUsage       float64
		tpuAlloc       float64
		tpuUsage       float64
		expectCPUScore int64
		expectMemScore int64
		expectGPUScore int64
		expectTPUScore int64
	}{
		{
			name:           "usage < alloc",
			cpuAlloc:       70,
			cpuUsage:       30,
			memAlloc:       1024 * 1024 * 1024 * 1024,
			memUsage:       1024 * 1024 * 1024 * 500,
			gpuAlloc:       8,
			gpuUsage:       4,
			tpuAlloc:       5,
			tpuUsage:       4,
			expectCPUScore: -20,
			expectMemScore: 2,
			expectGPUScore: -60,
			expectTPUScore: -90,
		},
		{
			name:           "usage = alloc",
			cpuAlloc:       70,
			cpuUsage:       70,
			memAlloc:       1024 * 1024 * 1024,
			memUsage:       1024 * 1024 * 1024,
			gpuAlloc:       8,
			gpuUsage:       8,
			tpuAlloc:       10,
			tpuUsage:       10,
			expectCPUScore: -100,
			expectMemScore: -100,
			expectGPUScore: -100,
			expectTPUScore: -100,
		},
		{
			name:           "usage > alloc",
			cpuAlloc:       70,
			cpuUsage:       80,
			memAlloc:       1024 * 1024 * 1024 * 1024,
			memUsage:       1024 * 1024 * 1024 * 1025,
			gpuAlloc:       8,
			gpuUsage:       10,
			tpuAlloc:       6,
			tpuUsage:       12,
			expectCPUScore: -100,
			expectMemScore: -100,
			expectGPUScore: -100,
			expectTPUScore: -100,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			score := Score{}
			cpuScore, memScore, gpuScore, tpuScore, err := score.normalizeScore(c.cpuAlloc, c.cpuUsage, c.memAlloc, c.memUsage, c.gpuAlloc, c.gpuUsage, c.tpuAlloc, c.tpuUsage)
			require.NoError(t, err)
			assert.Equal(t, c.expectCPUScore, cpuScore)
			assert.Equal(t, c.expectMemScore, memScore)
			assert.Equal(t, c.expectGPUScore, gpuScore)
			assert.Equal(t, c.expectTPUScore, tpuScore)
		})
	}
}

// Test calculateClusterAllocatable and calculateNodeResourceUsage
func TestCalculateClusterResources(t *testing.T) {
	// Create testing nodes and pods.
	node1 := &corev1.Node{
		ObjectMeta: v1.ObjectMeta{
			Name: "node1",
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:               resource.MustParse("16"),
				corev1.ResourceMemory:            resource.MustParse("32Gi"),
				corev1.ResourceName(ResourceGPU): resource.MustParse("6"),
			},
		},
	}

	node2 := &corev1.Node{
		ObjectMeta: v1.ObjectMeta{
			Name: "node2",
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:               resource.MustParse("32"),
				corev1.ResourceMemory:            resource.MustParse("64Gi"),
				corev1.ResourceName(ResourceGPU): resource.MustParse("8"),
			},
		},
	}

	testPod := &corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			// Mock Pod deployed in node2
			Labels: map[string]string{"name": "test"},
		},
		Spec: corev1.PodSpec{
			NodeName: "node2",
			Containers: []corev1.Container{
				{
					Name: "test",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:               resource.MustParse("4"),
							corev1.ResourceMemory:            resource.MustParse("8Gi"),
							corev1.ResourceName(ResourceGPU): resource.MustParse("2"),
						},
					},
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset(node1, node2, testPod)
	informerFactory := informers.NewSharedInformerFactory(clientset, 0)
	podInformer := informerFactory.Core().V1().Pods()
	nodeInformer := informerFactory.Core().V1().Nodes()
	podInformer.Informer().GetStore().Add(testPod)
	nodeInformer.Informer().GetStore().Add(node1)
	nodeInformer.Informer().GetStore().Add(node2)

	s := NewScore(nodeInformer, podInformer)

	// Test calculateClusterAllocatable
	gpuAlloc, nodeName, err := s.calculateClusterAllocatable(ResourceGPU)
	require.NoError(t, err)

	// Expect node2 has 8 GPUs.
	assert.Equal(t, float64(8), gpuAlloc)
	assert.Equal(t, "node2", nodeName)

	// Test calculateNodeResourceUsage
	gpuUsage, err := s.calculateNodeResourceUsage(nodeName, ResourceGPU)
	require.NoError(t, err)

	// Expect testPod use 2 GPUs.
	assert.Equal(t, float64(2), gpuUsage)
}
