package config

import (
	"testing"
)

func TestGetParsedAgentImage(t *testing.T) {
	testcases := []struct {
		agentImageName string
		expectErr      bool
		registry       string
		image          string
		tag            string
	}{
		{
			// no registry
			// no tag
			"open-cluster-management.io/otel-collector-agent",
			false,
			"open-cluster-management.io",
			"otel-collector-agent",
			"latest",
		},
		{
			// no tag
			"quay.io/open-cluster-management.io/otel-collector-agent",
			false,
			"quay.io/open-cluster-management.io",
			"otel-collector-agent",
			"latest",
		},
		{
			"quay.io/open-cluster-management.io/otel-collector-agent:v0.1.0",
			false,
			"quay.io/open-cluster-management.io",
			"otel-collector-agent",
			"v0.1.0",
		},
		{
			// registry with port
			"quay.io:443/open-cluster-management.io/otel-collector-agent:v0.1.0",
			false,
			"quay.io:443/open-cluster-management.io",
			"otel-collector-agent",
			"v0.1.0",
		},
		{
			// registry with port
			// no tag
			"quay.io:443/open-cluster-management.io/otel-collector-agent",
			false,
			"quay.io:443/open-cluster-management.io",
			"otel-collector-agent",
			"latest",
		},
		{
			// empty image name
			"",
			false,
			"quay.io/open-cluster-management.io",
			"otel-collector-agent",
			"latest",
		},
		{
			// wrong image name
			"foo",
			true,
			"",
			"",
			"",
		}, {
			// wrong image name
			"foo/foo/foo/foo",
			true,
			"",
			"",
			"",
		},
	}

	for _, c := range testcases {
		AgentImageName = c.agentImageName
		r, i, tag, err := GetParsedAgentImage("quay.io/open-cluster-management.io/otel-collector-agent")
		if err != nil {
			if c.expectErr {
				continue
			}
			t.Errorf("GetParsedAgentImage() error: %v", err)
		}

		if r != c.registry || i != c.image || tag != c.tag {
			t.Errorf("expect %s, %s, %s, but get %s, %s, %s", c.registry, c.image, c.tag, r, i, tag)
		}
	}
}
