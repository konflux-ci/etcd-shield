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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"

	etcd_shield "github.com/konflux-ci/etcd-shield/pkg"
)

var _ = Describe("Pkg/Metrics", func() {
	Describe("NewMetrics", func() {
		It("returns a Metrics value with a gauge named etcd_shield_alert_triggered", func() {
			m := etcd_shield.NewMetrics()
			Expect(m).NotTo(BeNil())
			Expect(m.Enabled).NotTo(BeNil())

			Expect(testutil.ToFloat64(m)).To(Equal(0.0))
		})
	})

	Describe("Collect", func() {
		It("emits the Enabled gauge value", func() {
			m := etcd_shield.NewMetrics()
			m.Enabled.Set(1)

			Expect(testutil.ToFloat64(m)).To(Equal(1.0))
		})

		It("sends one metric on the collect channel", func() {
			m := etcd_shield.NewMetrics()
			m.Enabled.Set(0.5)

			ch := make(chan prometheus.Metric, 4)
			m.Collect(ch)
			close(ch)

			var metrics []prometheus.Metric
			for met := range ch {
				metrics = append(metrics, met)
			}
			Expect(metrics).To(HaveLen(1))

			var pb dto.Metric
			Expect(metrics[0].Write(&pb)).To(Succeed())
			Expect(pb.Gauge.GetValue()).To(Equal(0.5))
		})
	})

	Describe("Describe", func() {
		It("describes the Enabled gauge", func() {
			m := etcd_shield.NewMetrics()
			ch := make(chan *prometheus.Desc, 4)
			m.Describe(ch)
			close(ch)

			var descs []*prometheus.Desc
			for d := range ch {
				descs = append(descs, d)
			}
			Expect(descs).To(HaveLen(1))
			Expect(descs[0].String()).To(ContainSubstring(`fqName: "etcd_shield_alert_triggered"`))
			Expect(descs[0].String()).To(ContainSubstring("Current state of etcd-shield"))
		})
	})
})
