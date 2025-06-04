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

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type Querier struct {
	state      StateManager
	prometheus PromQuery
	config     Config
	metrics    *Metrics
}

func NewQuerier(prom PromQuery, state StateManager, config Config, metrics *Metrics) *Querier {
	querier := Querier{
		prometheus: prom,
		state:      state,
		config:     config,
		metrics:    metrics,
	}

	return &querier
}

var _ manager.Runnable = &Querier{}
var _ manager.LeaderElectionRunnable = &Querier{}

func (q *Querier) NeedLeaderElection() bool {
	// for now, only one reader/writer to prometheus
	return true
}

func (q *Querier) Start(ctx context.Context) error {
	l := logr.FromContextOrDiscard(ctx)
	ticker := time.NewTicker(q.config.WaitTime.Duration)
	for {
		select {
		case <-ticker.C:
			err := q.Process(ctx)
			if err != nil {
				l.Error(err, "failed to process state")
			}
		case <-ctx.Done():
			return nil
		}
	}
}

func (q *Querier) Process(ctx context.Context) error {
	l := logr.FromContextOrDiscard(ctx)

	// step 1: check if the alert we're interested in is firing
	firing, err := q.prometheus.IsAlertFiring(ctx, q.config.Prometheus.AlertName)
	if err != nil {
		return err
	}
	l.Info("pipelinerun ingress status", "is-firing", firing)

	// step 2: update the webhooks
	err = q.state.WriteConfig(ctx, !firing)
	if err != nil {
		return err
	}

	// step 3: write metrics
	if firing {
		q.metrics.Enabled.Set(1.0)
	} else {
		q.metrics.Enabled.Set(0.0)
	}

	return nil
}
