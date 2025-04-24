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

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"os"

	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"github.com/go-logr/logr"
	shield "github.com/konflux-ci/etcd-shield/pkg"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

func namespace() string {
	return os.Getenv("NAMESPACE")
}

func SetupStateWithManager(manager manager.Manager, configPath string) error {
	client := manager.GetClient()
	cfg, err := shield.GetConfig(ctrl.Log, configPath)
	if err != nil {
		return fmt.Errorf("failed to fetch config: %s", err)
	}

	prom, err := shield.NewPrometheus(cfg.Prometheus.Address, cfg.Prometheus.Config)
	if err != nil {
		return fmt.Errorf("failed to setup prometheus connection: %s", err)
	}

	state := shield.NewState(client, types.NamespacedName{
		Namespace: cfg.DestNamespace,
		Name:      cfg.DestName,
	})

	querier := shield.NewQuerier(prom, state, *cfg)

	err = manager.Add(querier)
	if err != nil {
		return fmt.Errorf("failed to register prometheus querier: %s", err)
	}

	err = ctrl.NewWebhookManagedBy(manager).
		For(&tektonv1.PipelineRun{}).
		WithValidator(shield.NewWebhook(state)).
		Complete()
	if err != nil {
		ctrl.Log.Error(err, "unable to setup pipelinerun webhooks")
		os.Exit(1)
	}

	return nil
}

func loadTLSCert(l *logr.Logger, certPath, keyPath string) func(*tls.Config) {
	getCertificate := func(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
		cert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			l.Error(err, "Unable to load TLS certificates")
			return nil, fmt.Errorf("unable to load TLS certificates: %w", err)
		}

		return &cert, err
	}

	return func(config *tls.Config) {
		config.GetCertificate = getCertificate
	}
}

func main() {
	var enableLeaderElection bool
	var probeAddr string
	var webhookPort int
	var tlsCert string
	var tlsKey string
	var configPath string
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.IntVar(&webhookPort, "port", 9443, "Port to listen for webhook events on.")
	flag.StringVar(&tlsCert, "tls-cert", "/var/tls/tls.crt", "File location of tls certificate.")
	flag.StringVar(&tlsKey, "tls-key", "/var/tls/tls.key", "File location of tls key pair.")
	flag.StringVar(&configPath, "config", "/etc/etcd-shield/config.yaml", "Location of etcd-shield config")

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(tektonv1.AddToScheme(scheme))

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)

	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	ctx := logr.NewContext(context.Background(), ctrl.Log)

	tlsOpts := []func(*tls.Config){loadTLSCert(&ctrl.Log, tlsCert, tlsKey)}
	options := ctrl.Options{
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				// only watch for resources in this namespace
				namespace(): {},
			},
		},
		Scheme:                 scheme,
		Logger:                 ctrl.Log,
		BaseContext:            func() context.Context { return ctx },
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "etcd-shield.konflux-ci.dev",
		HealthProbeBindAddress: probeAddr,
		Metrics: server.Options{
			FilterProvider: filters.WithAuthenticationAndAuthorization,
			SecureServing:  true,
			TLSOpts:        tlsOpts,
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			Port:    webhookPort,
			TLSOpts: tlsOpts,
		}),
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		ctrl.Log.Error(err, "failed to create manager")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		ctrl.Log.Error(err, "unable to setup healthz check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		ctrl.Log.Error(err, "unable to setup readyz check")
		os.Exit(1)
	}

	if err := SetupStateWithManager(mgr, configPath); err != nil {
		ctrl.Log.Error(err, "failed to setup state with manager")
		os.Exit(1)
	}

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		ctrl.Log.Error(err, "failed to run manager")
		os.Exit(1)
	}
}
