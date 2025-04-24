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

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type State struct {
	client.Client
	ref types.NamespacedName
}

type StateManager interface {
	ReadConfig(context.Context) (bool, error)
	WriteConfig(context.Context, bool) error
}

func NewState(cli client.Client, ref types.NamespacedName) StateManager {
	return &State{
		Client: cli,
		ref:    ref,
	}
}

const CONFIG_KEY string = "allow"

func (s *State) WriteConfig(ctx context.Context, allow bool) error {
	configMap := v1.ConfigMap{}
	configMap.SetName(s.ref.Name)
	configMap.SetNamespace(s.ref.Namespace)
	_, err := controllerutil.CreateOrPatch(ctx, s.Client, &configMap, func() error {
		if configMap.Data == nil {
			configMap.Data = map[string]string{}
		}
		if allow {
			configMap.Data[CONFIG_KEY] = "1"
		} else {
			configMap.Data[CONFIG_KEY] = "0"
		}

		return nil
	})

	return err
}

func (s *State) ReadConfig(ctx context.Context) (bool, error) {
	configMap := v1.ConfigMap{}
	err := s.Get(ctx, s.ref, &configMap)
	if err != nil {
		if errors.IsNotFound(err) {
			// if no state is found, assume we're serving requests
			return true, nil
		}
		return false, err
	}

	data, ok := configMap.Data[CONFIG_KEY]
	if !ok {
		return true, nil
	}
	return data == "1", nil
}
