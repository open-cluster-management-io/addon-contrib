/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	dynamicscoringv1alpha1 "open-cluster-management.io/dynamic-scoring/api/v1alpha1"
	"open-cluster-management.io/dynamic-scoring/pkg/common"
)

var _ = Describe("DynamicScoringConfig helpers", func() {
	Context("mask helpers", func() {
		It("builds and checks mask map", func() {
			maskMap := buildMaskMap([]common.Mask{{ClusterName: "cluster-a", ScoreName: "latency"}})
			Expect(isMasked(maskMap, "cluster-a", "latency")).To(BeTrue())
			Expect(isMasked(maskMap, "cluster-a", "power")).To(BeFalse())
		})
	})

	Context("URL helpers", func() {
		It("validates URLs", func() {
			Expect(isValidURL("http://example.com/path")).To(BeTrue())
			Expect(isValidURL("://bad-url")).To(BeFalse())
		})
	})

	Context("manifest work builder", func() {
		It("embeds summaries in ConfigMap", func() {
			summaries := []common.ScorerSummary{{Name: "scorer-a", ScoreName: "latency"}}
			manifest := buildConfigManifestWork("cluster-a", summaries)
			Expect(manifest.Name).To(Equal(common.ManifestWorkConfigMapName))
			Expect(manifest.Namespace).To(Equal("cluster-a"))
			Expect(manifest.Spec.Workload.Manifests).To(HaveLen(1))

			cm, ok := manifest.Spec.Workload.Manifests[0].RawExtension.Object.(*corev1.ConfigMap)
			Expect(ok).To(BeTrue())
			Expect(cm.Name).To(Equal(common.DynamicScoringConfigName))
			Expect(cm.Namespace).To(Equal(common.DynamicScoringNamespace))
			Expect(cm.Data).To(HaveKey("summaries"))
		})
	})

	Context("endpoint helpers", func() {
		It("builds source endpoint from config URL", func() {
			scorer := dynamicscoringv1alpha1.DynamicScorer{
				Spec: dynamicscoringv1alpha1.DynamicScorerSpec{
					ConfigURL: "http://config.example.com/config",
					Source: dynamicscoringv1alpha1.SourceConfigWithAuth{
						Path: "/api/v1/query_range",
					},
				},
			}
			endpoint, err := getValidSourceFullEndpoint(scorer)
			Expect(err).NotTo(HaveOccurred())
			Expect(endpoint).To(Equal("http://config.example.com/api/v1/query_range"))
		})

		It("builds scoring endpoint from scoring host", func() {
			scorer := dynamicscoringv1alpha1.DynamicScorer{
				Spec: dynamicscoringv1alpha1.DynamicScorerSpec{
					ConfigURL: "http://config.example.com/config",
					Scoring: dynamicscoringv1alpha1.ScoringConfigWithAuth{
						Host: "http://score.example.com",
						Path: "/api/v1/score",
					},
				},
			}
			endpoint, err := getValidScoringFullEndpoint(scorer)
			Expect(err).NotTo(HaveOccurred())
			Expect(endpoint).To(Equal("http://score.example.com/api/v1/score"))
		})
	})

	Context("dimension format helper", func() {
		It("defaults when format is empty", func() {
			scorer := dynamicscoringv1alpha1.DynamicScorer{}
			format, err := getValidScoreDimentionFormat(scorer)
			Expect(err).NotTo(HaveOccurred())
			Expect(format).To(Equal("${scoreName}"))
		})
	})
})
