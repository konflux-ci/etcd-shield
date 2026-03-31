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

	etcd_shield "github.com/konflux-ci/etcd-shield/pkg"
)

var _ = Describe("Pkg/Webhook", func() {

	Describe("NewWebhook", func() {
		It("returns a non-nil admission.Webhook", func() {
			state := etcd_shield.NewInMemoryState()
			metrics := etcd_shield.NewMetrics()
			iface := etcd_shield.NewWebhook(state, metrics)
			Expect(iface).NotTo(BeNil())
		})
	})
})
