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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	etcd_shield "github.com/konflux-ci/etcd-shield/pkg"
)

var _ = Describe("Pkg/WebhookHandler", func() {
	var req admission.Request

	BeforeEach(func() {
		req = admission.Request{}
	})

	Describe("Handle", func() {
		It("allows the request when state reports allow", func(ctx context.Context) {
			state := etcd_shield.NewInMemoryState()
			Expect(state.WriteConfig(ctx, true)).To(Succeed())

			wh := etcd_shield.NewWebhook(state, etcd_shield.NewMetrics())
			resp := wh.Handler.Handle(ctx, req)

			Expect(resp.Allowed).To(BeTrue())
			Expect(resp.Result).NotTo(BeNil())
			Expect(resp.Result.Message).To(Equal("resource is allowed"))
		})

		It("denies the request when state reports deny", func(ctx context.Context) {
			state := etcd_shield.NewInMemoryState()
			Expect(state.WriteConfig(ctx, false)).To(Succeed())

			wh := etcd_shield.NewWebhook(state, etcd_shield.NewMetrics())
			resp := wh.Handler.Handle(ctx, req)

			Expect(resp.Allowed).To(BeFalse())
			Expect(resp.Result).NotTo(BeNil())
			Expect(resp.Result.Message).To(Equal("resource admission currently not allowed"))
		})

		It("returns an internal error response when ReadConfig fails", func(ctx context.Context) {
			rerr := errors.New("state read failed")
			wh := etcd_shield.NewWebhook(&mockStateManager{readErr: rerr}, etcd_shield.NewMetrics())
			resp := wh.Handler.Handle(ctx, req)

			Expect(resp.Allowed).To(BeFalse())
			Expect(resp.Result).NotTo(BeNil())
			Expect(resp.Result.Code).To(Equal(int32(500)))
			Expect(resp.Result.Message).To(Equal("state read failed"))
		})
	})
})
