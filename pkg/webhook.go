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
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type Webhook struct {
	state   StateManager
	metrics *Metrics
}

func NewWebhook(state StateManager, metrics *Metrics) admission.CustomValidator {
	return &Webhook{
		state:   state,
		metrics: metrics,
	}
}

var _ admission.CustomValidator = &Webhook{}

func (w *Webhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	allow, err := w.state.ReadConfig(ctx)
	if err != nil {
		return nil, err
	} else if !allow {
		return nil, fmt.Errorf("PipelineRun admission currently not allowed")
	}
	return nil, nil
}

func (*Webhook) ValidateDelete(context.Context, runtime.Object) (admission.Warnings, error) {
	// we don't care about validating deletes
	return nil, nil
}

func (*Webhook) ValidateUpdate(context.Context, runtime.Object, runtime.Object) (admission.Warnings, error) {
	// we don't care about validating updates
	return nil, nil
}
