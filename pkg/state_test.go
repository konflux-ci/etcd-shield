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
	"errors"

	etcd_shield "github.com/konflux-ci/etcd-shield/pkg"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// a dummy StateManager that can return an error from ReadConfig or WriteConfig
type mockStateManager struct {
	readErr  error
	writeErr error
}

func (e *mockStateManager) ReadConfig(context.Context) (bool, error) {
	return false, e.readErr
}

func (e *mockStateManager) WriteConfig(context.Context, bool) error {
	return e.writeErr
}

var _ = Describe("Pkg/State", func() {
	var _ = Describe("ReadConfig", func() {
		var k8sClient client.Client

		BeforeEach(func() {
			k8sClient = fake.NewClientBuilder().Build()
		})

		It("Should read a true state from the configmap", func(ctx context.Context) {
			configmap := v1.ConfigMap{
				Data: map[string]string{etcd_shield.CONFIG_KEY: "1"},
			}
			configmap.SetName("state")
			configmap.SetNamespace("etcd-shield")
			err := k8sClient.Create(ctx, &configmap)
			Expect(err).NotTo(HaveOccurred())

			state := etcd_shield.NewState(k8sClient, types.NamespacedName{Name: configmap.Name, Namespace: configmap.Namespace})
			allowed, err := state.ReadConfig(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeTrue())
		})

		It("Should read a false state from the configmap", func(ctx context.Context) {
			configmap := v1.ConfigMap{
				Data: map[string]string{etcd_shield.CONFIG_KEY: "0"},
			}
			configmap.SetName("state")
			configmap.SetNamespace("etcd-shield")
			err := k8sClient.Create(ctx, &configmap)
			Expect(err).NotTo(HaveOccurred())

			state := etcd_shield.NewState(k8sClient, types.NamespacedName{Name: configmap.Name, Namespace: configmap.Namespace})
			allowed, err := state.ReadConfig(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeFalse())
		})

		It("Should assume a default of true if the configmap isn't found", func(ctx context.Context) {
			state := etcd_shield.NewState(k8sClient, types.NamespacedName{Name: "state", Namespace: "etcd-shield"})
			allowed, err := state.ReadConfig(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeTrue())
		})

		It("Should assume a default of true if the allow key is missing from configmap data", func(ctx context.Context) {
			configmap := v1.ConfigMap{
				Data: map[string]string{"other": "value"},
			}
			configmap.SetName("state")
			configmap.SetNamespace("etcd-shield")
			Expect(k8sClient.Create(ctx, &configmap)).To(Succeed())

			state := etcd_shield.NewState(k8sClient, types.NamespacedName{Name: configmap.Name, Namespace: configmap.Namespace})
			allowed, err := state.ReadConfig(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeTrue())
		})

		It("Should return a non-nil error when getting the ConfigMap fails for a reason other than NotFound", func(ctx context.Context) {
			failingClient := fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					return errors.New("injected apiserver failure")
				},
			}).Build()

			state := etcd_shield.NewState(failingClient, types.NamespacedName{Name: "state", Namespace: "etcd-shield"})
			allowed, err := state.ReadConfig(ctx)
			Expect(err).To(MatchError("injected apiserver failure"))
			Expect(allowed).To(BeFalse())
		})
	})

	var _ = Describe("WriteConfig", func() {
		var k8sClient client.Client
		var ref types.NamespacedName

		BeforeEach(func() {
			k8sClient = fake.NewClientBuilder().Build()
			ref = types.NamespacedName{Name: "state", Namespace: "etcd-shield"}
		})
		exist := func() {
			configmap := v1.ConfigMap{
				Data: map[string]string{},
			}
			configmap.SetName(ref.Name)
			configmap.SetNamespace(ref.Namespace)
			err := k8sClient.Create(context.Background(), &configmap)
			Expect(err).NotTo(HaveOccurred())
		}

		DescribeTable("writes the given state to the ConfigMap", func(setup func(), allowed bool) {
			setup()
			state := etcd_shield.NewState(k8sClient, ref)
			err := state.WriteConfig(context.Background(), allowed)
			Expect(err).NotTo(HaveOccurred())

			read_state, err := state.ReadConfig(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(read_state).To(BeEquivalentTo(allowed))
		},
			Entry("existing configmap", exist, true),
			Entry("existing configmap", exist, false),
			Entry("non-existing configmap", func() {}, true),
			Entry("non-existing configmap", func() {}, false),
		)

		It("initializes nil Data when updating an existing configmap", func(ctx context.Context) {
			cm := &v1.ConfigMap{}
			cm.SetName(ref.Name)
			cm.SetNamespace(ref.Namespace)
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			state := etcd_shield.NewState(k8sClient, ref)
			Expect(state.WriteConfig(ctx, true)).To(Succeed())

			allowed, err := state.ReadConfig(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeTrue())
		})

		It("returns an error when Create fails for a missing configmap", func(ctx context.Context) {
			failingClient := fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
				Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					return errors.New("injected create failure")
				},
			}).Build()

			state := etcd_shield.NewState(failingClient, ref)
			err := state.WriteConfig(ctx, true)
			Expect(err).To(MatchError("injected create failure"))
		})

		It("returns an error when Patch fails for an existing configmap", func(ctx context.Context) {
			failingClient := fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
				Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
					return errors.New("injected patch failure")
				},
			}).Build()

			cm := &v1.ConfigMap{}
			cm.SetName(ref.Name)
			cm.SetNamespace(ref.Namespace)
			cm.Data = map[string]string{etcd_shield.CONFIG_KEY: "0"}
			Expect(failingClient.Create(ctx, cm)).To(Succeed())

			state := etcd_shield.NewState(failingClient, ref)
			err := state.WriteConfig(ctx, true)
			Expect(err).To(MatchError("injected patch failure"))
		})
	})
})
