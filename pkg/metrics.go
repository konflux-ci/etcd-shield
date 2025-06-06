// Copyright 2025 Red Hat Inc.
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

package etcd_shield

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	Denied  prometheus.Counter
	Allowed prometheus.Counter
	Enabled prometheus.Gauge
}

var _ prometheus.Collector = &Metrics{}

// Collect implements prometheus.Collector.
func (m *Metrics) Collect(channel chan<- prometheus.Metric) {
	m.Denied.Collect(channel)
	m.Allowed.Collect(channel)
	m.Enabled.Collect(channel)
}

// Describe implements prometheus.Collector.
func (m *Metrics) Describe(channel chan<- *prometheus.Desc) {
	m.Denied.Describe(channel)
	m.Allowed.Describe(channel)
	m.Enabled.Describe(channel)
}

func NewMetrics() *Metrics {
	return &Metrics{
		Denied: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "etcd_shield",
			Subsystem: "webhook",
			Name:      "denied_resources",
			Help:      "Number of resources that have been denied admission by `etcd-shield`",
		}),
		Allowed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "etcd_shield",
			Subsystem: "webhook",
			Name:      "allowed_resources",
			Help:      "Number of resources that have been allowed admission by `etcd-shield`",
		}),
		Enabled: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "etcd_shield",
			Subsystem: "alert",
			Name:      "triggered",
			Help:      "Current state of etcd-shield, as observed by etcd-shield itself.  0 implies allowing, 1 implies denying.",
		}),
	}
}
