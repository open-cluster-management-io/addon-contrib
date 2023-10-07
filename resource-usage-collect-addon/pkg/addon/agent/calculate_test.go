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

func TestNormalizeValue(t *testing.T) {
	cases := []struct {
		name           string
		cpuAlloc       float64
		cpuUsage       float64
		memAlloc       float64
		memUsage       float64
		expectCPUScore int64
		expectMemScore int64
	}{
		{
			name:           "usage < alloc",
			cpuAlloc:       70,
			cpuUsage:       30,
			memAlloc:       1024 * 1024 * 1024 * 1024,
			memUsage:       1024 * 1024 * 1024 * 500,
			expectCPUScore: -20,
			expectMemScore: 2,
		},
		{
			name:           "usage = alloc",
			cpuAlloc:       70,
			cpuUsage:       70,
			memAlloc:       1024 * 1024 * 1024,
			memUsage:       1024 * 1024 * 1024,
			expectCPUScore: -100,
			expectMemScore: -100,
		},
		{
			name:           "usage > alloc",
			cpuAlloc:       70,
			cpuUsage:       80,
			memAlloc:       1024 * 1024 * 1024 * 1024,
			memUsage:       1024 * 1024 * 1024 * 1025,
			expectCPUScore: -100,
			expectMemScore: -100,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			score := Score{}
			cpuScore, memScore, err := score.normalizeScore(c.cpuAlloc, c.cpuUsage, c.memAlloc, c.memUsage)
			require.NoError(t, err)
			assert.Equal(t, c.expectCPUScore, cpuScore)
			assert.Equal(t, c.expectMemScore, memScore)
		})
	}
}

func TestCalculatePodResourceRequest(t *testing.T) {
	testPod := &corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "test",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset()
	informerFactory := informers.NewSharedInformerFactory(clientset, 0)
	podInformer := informerFactory.Core().V1().Pods()
	nodeInformer := informerFactory.Core().V1().Nodes()
	podInformer.Informer().GetStore().Add(testPod)

	s := NewScore(nodeInformer, podInformer)

	cpuRequest, err := s.calculatePodResourceRequest(corev1.ResourceCPU)
	require.NoError(t, err)

	cpuExpected := 0.5
	assert.Equal(t, cpuExpected, cpuRequest)

	memoryRequest, err := s.calculatePodResourceRequest(corev1.ResourceMemory)
	require.NoError(t, err)

	memoryExpected := float64(1073741824) // 1GiB
	assert.Equal(t, memoryExpected, memoryRequest)
}
