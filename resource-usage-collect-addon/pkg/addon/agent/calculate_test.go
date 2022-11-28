package agent

import (
	"testing"
)

func TestNormalizeValue(t *testing.T) {
	cases := []struct {
		name           string
		cpuAlloc       int64
		cpuUsage       int64
		memAlloc       int64
		memUsage       int64
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
			cpuScore, memScore, _ := score.normalizeScore(c.cpuAlloc, c.cpuUsage, c.memAlloc, c.memUsage)

			if cpuScore != c.expectCPUScore {
				t.Errorf("expected cpuScore %v, but got %v", c.expectCPUScore, cpuScore)
			}

			if memScore != c.expectMemScore {
				t.Errorf("expected memScore %v, but got %v", c.expectMemScore, memScore)
			}
		})
	}
}
