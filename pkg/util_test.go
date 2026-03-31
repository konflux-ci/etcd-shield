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
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	etcd_shield "github.com/konflux-ci/etcd-shield/pkg"
)

var _ = Describe("Pkg/Util", func() {
	Describe("NewDuration", func() {
		It("wraps a time.Duration", func() {
			d := etcd_shield.NewDuration(12 * time.Minute)
			Expect(d.Duration).To(Equal(12 * time.Minute))
		})
	})

	Describe("MarshalJSON", func() {
		It("encodes as a JSON string of the Go duration form", func() {
			d := etcd_shield.NewDuration(15 * time.Second)
			b, err := json.Marshal(d)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(Equal(`"15s"`))
		})
	})

	Describe("UnmarshalJSON", func() {
		It("decodes a JSON string with time.ParseDuration semantics", func() {
			var d etcd_shield.Duration
			err := json.Unmarshal([]byte(`"2h30m"`), &d)
			Expect(err).NotTo(HaveOccurred())
			Expect(d.Duration).To(Equal(2*time.Hour + 30*time.Minute))
		})

		It("decodes a JSON number as nanoseconds", func() {
			var d etcd_shield.Duration
			err := json.Unmarshal([]byte(`3000000000`), &d)
			Expect(err).NotTo(HaveOccurred())
			Expect(d.Duration).To(Equal(3 * time.Second))
		})

		It("returns an error for a non-duration string", func() {
			var d etcd_shield.Duration
			err := json.Unmarshal([]byte(`"destName: [\n  unclosed"`), &d)
			Expect(err).To(HaveOccurred())
		})

		It("returns an error for invalid JSON", func() {
			var d etcd_shield.Duration
			err := json.Unmarshal([]byte(`not json }`), &d)
			Expect(err).To(HaveOccurred())
		})

		It("returns an error when the JSON value is not a string or number", func() {
			var d etcd_shield.Duration
			err := json.Unmarshal([]byte(`true`), &d)
			Expect(err).To(MatchError(ContainSubstring("invalid duration")))
		})
	})
})
