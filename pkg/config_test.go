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

package etcd_shield_test

import (
	"os"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"

	etcd_shield "github.com/konflux-ci/etcd-shield/pkg"
)

var _ = Describe("Pkg/Config", func() {
	var yamlConfig string

	BeforeEach(func() {
		cfg := etcd_shield.Config{
			DestName:      "etcd-shield-state",
			DestNamespace: "etcd-shield",
			Prometheus: etcd_shield.PrometheusConfig{
				AlertName: "foo",
				Address:   "prometheus.prometheus.svc:8080",
			},
			WaitTime: etcd_shield.NewDuration(15 * time.Second),
		}
		config, err := yaml.Marshal(&cfg)
		Expect(err).NotTo(HaveOccurred())

		yamlConfig = string(config)
	})

	Describe("GetConfig", func() {
		It("loads and returns config from a file path", func() {
			f, err := os.CreateTemp("", "etcd-shield-config-*.yaml")
			Expect(err).NotTo(HaveOccurred())
			defer func() { Expect(os.Remove(f.Name())).To(Succeed()) }()

			_, err = f.WriteString(yamlConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(f.Close()).To(Succeed())

			cfg, err := etcd_shield.GetConfig(logr.Discard(), f.Name())
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())

			Expect(cfg.DestName).To(Equal("etcd-shield-state"))
			Expect(cfg.DestNamespace).To(Equal("etcd-shield"))
			Expect(cfg.Prometheus.AlertName).To(Equal("foo"))
			Expect(cfg.Prometheus.Address).To(Equal("prometheus.prometheus.svc:8080"))
			Expect(cfg.WaitTime).To(Equal(etcd_shield.NewDuration(15 * time.Second)))
		})

		It("returns an error when the file does not exist", func() {
			cfg, err := etcd_shield.GetConfig(logr.Discard(), "/nonexistent/path/etcd-shield-config.yaml")
			Expect(err).To(HaveOccurred())
			Expect(cfg).To(BeNil())
		})

		It("returns an error when the file is not valid YAML", func() {
			f, err := os.CreateTemp("", "etcd-shield-bad-config-*.yaml")
			Expect(err).NotTo(HaveOccurred())
			defer func() { Expect(os.Remove(f.Name())).To(Succeed()) }()

			_, err = f.WriteString("destName: [\n  unclosed")
			Expect(err).NotTo(HaveOccurred())
			Expect(f.Close()).To(Succeed())

			cfg, err := etcd_shield.GetConfig(logr.Discard(), f.Name())
			Expect(err).To(HaveOccurred())
			Expect(cfg).To(BeNil())
		})
	})
})
