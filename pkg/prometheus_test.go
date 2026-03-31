// Copyright 2026 Red Hat Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package etcd_shield_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/config"

	etcd_shield "github.com/konflux-ci/etcd-shield/pkg"
)

// prometheusAPIResponse returns a marshalledJSON response for the Prometheus API.
func prometheusAPIResponse(alerts []map[string]any) []byte {
	if alerts == nil {
		alerts = []map[string]any{}
	}
	body := map[string]any{
		"status": "success",
		"data": map[string]any{
			"alerts": alerts,
		},
	}
	b, err := json.Marshal(body)
	Expect(err).NotTo(HaveOccurred())
	return b
}

var _ = Describe("Pkg/Prometheus", func() {
	Describe("NewPrometheus", func() {
		It("returns a PromQuery client for a valid address and HTTP config", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(prometheusAPIResponse(nil))
			}))
			defer srv.Close()

			pq, err := etcd_shield.NewPrometheus(srv.URL, config.HTTPClientConfig{})
			Expect(err).NotTo(HaveOccurred())
			Expect(pq).NotTo(BeNil())
		})

		It("returns an error when HTTPClientConfig cannot build an HTTP client", func() {
			cfg := config.HTTPClientConfig{
				TLSConfig: config.TLSConfig{
					MinVersion: config.TLSVersions["TLS13"],
					MaxVersion: config.TLSVersions["TLS12"],
				},
			}
			_, err := etcd_shield.NewPrometheus("http://127.0.0.1:9090", cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("tls_config.max_version must be greater than or equal to tls_config.min_version"))
		})

		It("returns an error when the address is not a valid URL", func() {
			_, err := etcd_shield.NewPrometheus("http://%zz", config.HTTPClientConfig{})
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("IsAlertFiring", func() {
		var ctx context.Context

		BeforeEach(func() {
			ctx = context.Background()
		})

		It("returns true when a firing alert matches the given alert name", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/api/v1/alerts"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(prometheusAPIResponse([]map[string]any{
					{
						"labels": map[string]any{"alertname": "TestAlert"},
						"state":  string(v1.AlertStateFiring),
					},
				}))
			}))
			defer srv.Close()

			pq, err := etcd_shield.NewPrometheus(srv.URL, config.HTTPClientConfig{})
			Expect(err).NotTo(HaveOccurred())

			firing, err := pq.IsAlertFiring(ctx, "TestAlert")
			Expect(err).NotTo(HaveOccurred())
			Expect(firing).To(BeTrue())
		})

		It("returns false when the alert name matches but state is not firing", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(prometheusAPIResponse([]map[string]any{
					{
						"labels": map[string]any{"alertname": "TestAlert"},
						"state":  string(v1.AlertStatePending),
					},
				}))
			}))
			defer srv.Close()

			pq, err := etcd_shield.NewPrometheus(srv.URL, config.HTTPClientConfig{})
			Expect(err).NotTo(HaveOccurred())

			firing, err := pq.IsAlertFiring(ctx, "TestAlert")
			Expect(err).NotTo(HaveOccurred())
			Expect(firing).To(BeFalse())
		})

		It("returns false when no firing alert matches the given alert name", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(prometheusAPIResponse([]map[string]any{
					{
						"labels": map[string]any{"alertname": "TestAlert"},
						"state":  string(v1.AlertStateFiring),
					},
				}))
			}))
			defer srv.Close()

			pq, err := etcd_shield.NewPrometheus(srv.URL, config.HTTPClientConfig{})
			Expect(err).NotTo(HaveOccurred())

			firing, err := pq.IsAlertFiring(ctx, "OtherAlert")
			Expect(err).NotTo(HaveOccurred())
			Expect(firing).To(BeFalse())
		})

		It("returns false when there are no alerts", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(prometheusAPIResponse(nil))
			}))
			defer srv.Close()

			pq, err := etcd_shield.NewPrometheus(srv.URL, config.HTTPClientConfig{})
			Expect(err).NotTo(HaveOccurred())

			firing, err := pq.IsAlertFiring(ctx, "TestAlert")
			Expect(err).NotTo(HaveOccurred())
			Expect(firing).To(BeFalse())
		})

		It("returns an error when the alerts request fails", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "internal error", http.StatusInternalServerError)
			}))
			defer srv.Close()

			pq, err := etcd_shield.NewPrometheus(srv.URL, config.HTTPClientConfig{})
			Expect(err).NotTo(HaveOccurred())

			firing, err := pq.IsAlertFiring(ctx, "TestAlert")
			Expect(err).To(HaveOccurred())
			Expect(firing).To(BeFalse())
		})
	})
})
