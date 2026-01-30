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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	dynamicscoringv1alpha1 "open-cluster-management.io/dynamic-scoring/api/v1alpha1"
	"open-cluster-management.io/dynamic-scoring/pkg/common"
)

var _ = Describe("DynamicScorer helpers", func() {
	Context("syncScoringHealthz", func() {
		It("marks scorer active on 200", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/healthz" {
					w.WriteHeader(http.StatusOK)
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			defer server.Close()

			scorer := &dynamicscoringv1alpha1.DynamicScorer{
				Spec: dynamicscoringv1alpha1.DynamicScorerSpec{
					ConfigURL: server.URL + "/config",
				},
			}

			err := syncScoringHealthz(context.Background(), scorer)
			Expect(err).NotTo(HaveOccurred())
			Expect(scorer.Status.HealthStatus).To(Equal(common.ScorerHealthStatusActive))
		})

		It("marks scorer inactive on non-200", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/healthz" {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			defer server.Close()

			scorer := &dynamicscoringv1alpha1.DynamicScorer{
				Spec: dynamicscoringv1alpha1.DynamicScorerSpec{
					ConfigURL: server.URL + "/config",
				},
			}

			err := syncScoringHealthz(context.Background(), scorer)
			Expect(err).NotTo(HaveOccurred())
			Expect(scorer.Status.HealthStatus).To(Equal(common.ScorerHealthStatusInactive))
		})
	})

	Context("syncScoringConfig", func() {
		It("updates LastSyncedConfig from endpoint", func() {
			expected := common.Config{
				Name:        "sample-score",
				Description: "sample",
				Source: common.SourceConfig{
					Type: common.SourceTypePrometheus,
					Host: "http://prometheus.example.com",
					Path: "/api/v1/query_range",
					Params: common.SourceParams{
						Query: "up",
						Range: 60,
						Step:  5,
					},
				},
				Scoring: common.ScoringConfig{
					Host: "http://scoring.example.com",
					Path: "/api/v1/score",
					Params: common.ScoringParams{
						Name:     "sample-score",
						Interval: 30,
					},
				},
			}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/config" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(expected); err != nil {
					w.WriteHeader(http.StatusInternalServerError)
				}
			}))
			defer server.Close()

			scorer := &dynamicscoringv1alpha1.DynamicScorer{
				Spec: dynamicscoringv1alpha1.DynamicScorerSpec{
					ConfigURL: server.URL + "/config",
				},
			}

			err := syncScoringConfig(context.Background(), scorer)
			Expect(err).NotTo(HaveOccurred())
			Expect(scorer.Status.HealthStatus).To(Equal(common.ScorerHealthStatusActive))
			Expect(scorer.Status.LastSyncedConfig).NotTo(BeNil())
			Expect(*scorer.Status.LastSyncedConfig).To(Equal(expected))
		})
	})
})
