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

package main

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	shield "github.com/konflux-ci/etcd-shield/pkg"
)

func TestEtcdShieldCmd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "etcd-shield cmd")
}

var _ = Describe("namespace", func() {
	It("returns the value of the NAMESPACE environment variable", func() {
		Expect(os.Setenv("NAMESPACE", "my-team-ns")).To(Succeed())
		DeferCleanup(func() { Expect(os.Unsetenv("NAMESPACE")).To(Succeed()) })

		Expect(namespace()).To(Equal("my-team-ns"))
	})

	It("returns empty string when NAMESPACE is unset", func() {
		Expect(os.Unsetenv("NAMESPACE")).To(Succeed())
		Expect(namespace()).To(Equal(""))
	})
})

var _ = Describe("stateManager", func() {
	BeforeEach(func() {
		ctrl.SetLogger(logr.Discard())
	})

	It("returns an in-memory state object when STATE_MANAGER is 'in-memory'", func() {
		Expect(os.Setenv("STATE_MANAGER", "in-memory")).To(Succeed())
		DeferCleanup(func() { Expect(os.Unsetenv("STATE_MANAGER")).To(Succeed()) })

		cli := fake.NewClientBuilder().Build()
		cfg := &shield.Config{DestName: "s", DestNamespace: "ns"}
		sm := stateManager(cli, cfg)

		Expect(reflect.TypeOf(sm).String()).To(ContainSubstring("InMemoryState"))
	})

	It("returns configmap-backed state object when STATE_MANAGER is not in-memory", func() {
		Expect(os.Setenv("STATE_MANAGER", "anything-else")).To(Succeed())
		DeferCleanup(func() { Expect(os.Unsetenv("STATE_MANAGER")).To(Succeed()) })

		cli := fake.NewClientBuilder().Build()
		cfg := &shield.Config{DestName: "state", DestNamespace: "etcd-shield"}
		sm := stateManager(cli, cfg)

		Expect(reflect.TypeOf(sm).String()).To(ContainSubstring("State"))
		Expect(reflect.TypeOf(sm).String()).NotTo(ContainSubstring("InMemoryState"))

		_, err := sm.ReadConfig(context.Background())
		Expect(err).NotTo(HaveOccurred())
	})
})

type mockWebhookServer struct {
	registeredPath    string
	registeredHandler http.Handler
}

func (s *mockWebhookServer) NeedLeaderElection() bool { return false }

func (s *mockWebhookServer) Register(path string, h http.Handler) {
	s.registeredPath = path
	s.registeredHandler = h
}

func (s *mockWebhookServer) Start(ctx context.Context) error { return nil }

func (s *mockWebhookServer) StartedChecker() healthz.Checker {
	return func(*http.Request) error { return nil }
}

func (s *mockWebhookServer) WebhookMux() *http.ServeMux { return http.NewServeMux() }

// mockManagerState is a narrow mock of manager.Manager that implements the managerState interface.
type mockManagerState struct {
	k8sCli client.Client
	addErr error
	srv    *mockWebhookServer
}

func (m *mockManagerState) GetClient() client.Client { return m.k8sCli }

func (m *mockManagerState) Add(_ manager.Runnable) error { return m.addErr }

func (m *mockManagerState) GetWebhookServer() webhook.Server { return m.srv }

var _ = Describe("SetupStateWithManager", Ordered, func() {
	BeforeEach(func() {
		ctrl.SetLogger(logr.Discard())
	})

	It("returns an error when the config file cannot be read", func() {
		mgr := &mockManagerState{
			k8sCli: fake.NewClientBuilder().Build(),
			srv:    &mockWebhookServer{},
		}
		err := SetupStateWithManager(mgr, "/nonexistent/etcd-shield-config.yaml")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to fetch config"))
	})

	It("returns an error when prometheus client config is invalid", func() {
		dir := GinkgoT().TempDir()
		configPath := filepath.Join(dir, "config.yaml")
		cfgYAML := `destName: state
destNamespace: ns
prometheus:
  address: "http://%zz"
  alertName: alert
waitTime: 100ms
`
		Expect(os.WriteFile(configPath, []byte(cfgYAML), 0o600)).To(Succeed())

		mgr := &mockManagerState{
			k8sCli: fake.NewClientBuilder().Build(),
			srv:    &mockWebhookServer{},
		}
		err := SetupStateWithManager(mgr, configPath)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to setup prometheus connection"))
	})

	It("returns an error when the manager rejects the prometheus querier", func() {
		Expect(os.Setenv("STATE_MANAGER", "in-memory")).To(Succeed())
		DeferCleanup(func() { Expect(os.Unsetenv("STATE_MANAGER")).To(Succeed()) })

		dir := GinkgoT().TempDir()
		configPath := filepath.Join(dir, "config.yaml")
		cfgYAML := `destName: state
destNamespace: ns
prometheus:
  address: http://127.0.0.1:9090
  alertName: alert
waitTime: 100ms
`
		Expect(os.WriteFile(configPath, []byte(cfgYAML), 0o600)).To(Succeed())

		srv := &mockWebhookServer{}
		mgr := &mockManagerState{
			k8sCli: fake.NewClientBuilder().Build(),
			srv:    srv,
			addErr: errors.New("manager rejected querier"),
		}
		err := SetupStateWithManager(mgr, configPath)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to register prometheus querier"))
		Expect(srv.registeredPath).To(Equal(""))
		Expect(srv.registeredHandler).To(BeNil())
	})

	It("registers the querier, metrics, and validating webhook when setup succeeds", func() {
		Expect(os.Setenv("STATE_MANAGER", "in-memory")).To(Succeed())
		DeferCleanup(func() { Expect(os.Unsetenv("STATE_MANAGER")).To(Succeed()) })

		dir := GinkgoT().TempDir()
		configPath := filepath.Join(dir, "config.yaml")
		cfgYAML := `destName: state
destNamespace: ns
prometheus:
  address: http://127.0.0.1:9090
  alertName: alert
waitTime: 100ms
`
		Expect(os.WriteFile(configPath, []byte(cfgYAML), 0o600)).To(Succeed())

		srv := &mockWebhookServer{}
		mgr := &mockManagerState{
			k8sCli: fake.NewClientBuilder().Build(),
			srv:    srv,
		}
		Expect(SetupStateWithManager(mgr, configPath)).To(Succeed())

		Expect(srv.registeredPath).To(Equal("/validate-resource"))
		Expect(srv.registeredHandler).NotTo(BeNil())
	})
})

var _ = Describe("loadTLSCert", func() {
	It("installs a GetCertificate that returns an error when cert files are missing", func() {
		logger := logr.Discard()
		opt := loadTLSCert(&logger, "/nonexistent/etcd-shield.crt", "/nonexistent/etcd-shield.key")

		cfg := &tls.Config{}
		opt(cfg)
		Expect(cfg.GetCertificate).NotTo(BeNil())

		cert, err := cfg.GetCertificate(&tls.ClientHelloInfo{})
		Expect(err).To(HaveOccurred())
		Expect(cert).To(BeNil())
	})
})
