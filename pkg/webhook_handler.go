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

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var _ admission.Handler = &Handler{}

type Handler struct {
	state   StateManager
	metrics *Metrics
}

func (w *Handler) Handle(ctx context.Context, _ admission.Request) admission.Response {
	allow, err := w.state.ReadConfig(ctx)
	if err != nil {
		return admission.Errored(500, err)
	}

	if !allow {
		return admission.Denied("admission currently not allowed")
	}
	return admission.Allowed("object is allowed")
}
