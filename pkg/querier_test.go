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
	"errors"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"

	etcd_shield "github.com/konflux-ci/etcd-shield/pkg"
)

type mockProm struct {
	firing    bool
	err       error
	calls     atomic.Int64
	mu        sync.Mutex
	lastAlert string
}

func (s *mockProm) IsAlertFiring(_ context.Context, alertName string) (bool, error) {
	s.calls.Add(1)
	s.mu.Lock()
	s.lastAlert = alertName
	s.mu.Unlock()
	return s.firing, s.err
}

func (s *mockProm) callCount() int {
	return int(s.calls.Load())
}

func (s *mockProm) lastAlertName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastAlert
}

func mockQuerierConfig() etcd_shield.Config {
	return etcd_shield.Config{
		WaitTime: etcd_shield.NewDuration(10 * time.Millisecond),
		Prometheus: etcd_shield.PrometheusConfig{
			AlertName: "test-alert",
		},
	}
}

var _ = Describe("Pkg/Querier", func() {
	Describe("NewQuerier", func() {
		It("returns a querier wired with the given dependencies", func() {
			prom := &mockProm{}
			state := etcd_shield.NewInMemoryState()
			m := etcd_shield.NewMetrics()
			q := etcd_shield.NewQuerier(prom, state, mockQuerierConfig(), m)

			Expect(q).NotTo(BeNil())
			Expect(prom.lastAlertName()).To(Equal(""))
			Expect(prom.callCount()).To(Equal(0))
		})
	})

	Describe("NeedLeaderElection", func() {
		It("returns true", func() {
			q := etcd_shield.NewQuerier(&mockProm{}, etcd_shield.NewInMemoryState(), mockQuerierConfig(), etcd_shield.NewMetrics())
			Expect(q.NeedLeaderElection()).To(BeTrue())
		})
	})

	Describe("Process", func() {
		It("writes allow=false and sets metric to 1 when the alert is firing", func() {
			ctx := context.Background()
			prom := &mockProm{firing: true}
			state := etcd_shield.NewInMemoryState()
			m := etcd_shield.NewMetrics()
			q := etcd_shield.NewQuerier(prom, state, mockQuerierConfig(), m)

			Expect(q.Process(ctx)).To(Succeed())

			allowed, err := state.ReadConfig(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeFalse())
			Expect(testutil.ToFloat64(m)).To(Equal(1.0))
		})

		It("writes allow=true and sets metric to 0 when the alert is not firing", func() {
			ctx := context.Background()
			prom := &mockProm{firing: false}
			state := etcd_shield.NewInMemoryState()
			m := etcd_shield.NewMetrics()
			q := etcd_shield.NewQuerier(prom, state, mockQuerierConfig(), m)

			Expect(q.Process(ctx)).To(Succeed())

			allowed, err := state.ReadConfig(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeTrue())
			Expect(testutil.ToFloat64(m)).To(Equal(0.0))
		})

		It("returns an error when prometheus fails to check state of alerts", func() {
			ctx := context.Background()
			prom := &mockProm{err: errors.New("prometheus unavailable")}
			state := etcd_shield.NewInMemoryState()
			m := etcd_shield.NewMetrics()
			q := etcd_shield.NewQuerier(prom, state, mockQuerierConfig(), m)

			err := q.Process(ctx)
			Expect(err).To(MatchError("prometheus unavailable"))

			allowed, rerr := state.ReadConfig(ctx)
			Expect(rerr).NotTo(HaveOccurred())
			Expect(allowed).To(BeFalse())
			Expect(testutil.ToFloat64(m)).To(Equal(0.0))
		})

		It("returns an error when writing to state fails", func() {
			ctx := context.Background()
			prom := &mockProm{firing: false}
			werr := errors.New("write failed")
			state := &mockStateManager{writeErr: werr}
			m := etcd_shield.NewMetrics()
			q := etcd_shield.NewQuerier(prom, state, mockQuerierConfig(), m)

			err := q.Process(ctx)
			Expect(err).To(MatchError(werr))
			Expect(testutil.ToFloat64(m)).To(Equal(0.0))
		})
	})

	Describe("Start", func() {
		It("runs Process on the ticker until the context is cancelled", func() {
			prom := &mockProm{firing: false}
			state := etcd_shield.NewInMemoryState()
			m := etcd_shield.NewMetrics()
			q := etcd_shield.NewQuerier(prom, state, mockQuerierConfig(), m)

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				defer close(done)
				_ = q.Start(ctx)
			}()

			// Should run about 20 times since the ticker is set for 10 ms
			Eventually(prom.callCount).WithTimeout(209 * time.Millisecond).WithPolling(5 * time.Millisecond).Should(BeNumerically(">=", 20))
			cancel()
			Eventually(done).WithTimeout(1 * time.Second).Should(BeClosed())
		})
	})
})
