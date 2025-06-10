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
	"context"

	etcd_shield "github.com/konflux-ci/etcd-shield/pkg"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"pgregory.net/rapid"
)

var _ = Describe("in-memory smoke tests", func() {
	var state etcd_shield.StateManager

	BeforeEach(func() {
		state = etcd_shield.NewInMemoryState()
	})

	It("should return the value written", func(ctx context.Context) {
		rapid.Check(GinkgoT(), func(t *rapid.T) {
			g := NewWithT(t)

			for range 100 {
				s := rapid.Bool().Draw(t, "state")

				g.Expect(state.WriteConfig(ctx, s)).To(Succeed())
				g.Expect(state.ReadConfig(ctx)).To(Equal(s))
			}
		})
	})

	It("should not return errors on reads", func(ctx context.Context) {
		rapid.Check(GinkgoT(), func(t *rapid.T) {
			g := NewWithT(t)
			s := rapid.Bool().Draw(t, "state")

			g.Expect(state.WriteConfig(ctx, s)).To(Succeed())

			for range 100 {
				g.Expect(state.ReadConfig(ctx)).To(Equal(s))
			}
		})
	})

	It("should not return errors on writes", func(ctx context.Context) {
		rapid.Check(GinkgoT(), func(t *rapid.T) {
			g := NewWithT(t)
			s := rapid.Bool().Draw(t, "state")

			for range 100 {
				g.Expect(state.WriteConfig(ctx, s)).To(Succeed())
			}
		})
	})
})
