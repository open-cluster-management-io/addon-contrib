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

// Test normalizeScore function.
func TestNormalizeValue(t *testing.T) {
	cases := []struct {
		name           string
		cpuAvailable   float64
		memAvailable   float64
		gpuAvailable   float64
		tpuAvailable   float64
		expectCPUScore int32
		expectMemScore int32
		expectGPUScore int32
		expectTPUScore int32
	}{
		{
			name:           "usage = alloc", // Indicating that cpuAvailable, gpuAvailable etc. are all 0.
			cpuAvailable:   0,
			memAvailable:   0,
			gpuAvailable:   0,
			tpuAvailable:   0,
			expectCPUScore: -100,
			expectMemScore: -100,
			expectGPUScore: -100,
			expectTPUScore: -100,
		},
		{
			name:           "usage < alloc", // Indicating that cpuAvailable, gpuAvailable etc. are all positive.
			cpuAvailable:   40,
			memAvailable:   524 * 1024 * 1024 * 1024,
			gpuAvailable:   2,
			tpuAvailable:   1,
			expectCPUScore: -20,
			expectMemScore: 100,
			expectGPUScore: -80,
			expectTPUScore: -90,
		},
		{
			name:           "usage > alloc", // Indicating that cpuAvailable, gpuAvailable etc. are all negative.
			cpuAvailable:   -10,
			memAvailable:   -1024 * 1024 * 1024,
			gpuAvailable:   -10,
			tpuAvailable:   -10,
			expectCPUScore: -100,
			expectMemScore: -100,
			expectGPUScore: -100,
			expectTPUScore: -100,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			score := Score{}
			cpuScore, memScore, gpuScore, tpuScore, err := score.normalizeScore("testScope", c.cpuAvailable, c.memAvailable, c.gpuAvailable, c.tpuAvailable)
			require.NoError(t, err)
			assert.Equal(t, c.expectCPUScore, cpuScore)
			assert.Equal(t, c.expectMemScore, memScore)
			assert.Equal(t, c.expectGPUScore, gpuScore)
			assert.Equal(t, c.expectTPUScore, tpuScore)
		})
	}
}

// Test the calculation of resources across the cluster and on specific nodes
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

	// Test calculateClusterAvailable for GPUs
	totalGPUAvailable, err := s.calculateClusterAvailable(ResourceGPU)
	require.NoError(t, err)

	// The cluster should have 12 GPUs available (6 from node1 + 6 from node2 after deducting 2 used by testPod).
	assert.Equal(t, float64(12), totalGPUAvailable)

	// Test calculateNodeResourceUsage for node2
	gpuUsage, err := s.calculateNodeResourceUsage("node2", ResourceGPU)
	require.NoError(t, err)

	// Expect testPod on node2 to use 2 GPUs.
	assert.Equal(t, float64(2), gpuUsage)
}
