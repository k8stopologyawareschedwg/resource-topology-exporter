/*
 * Copyright 2024 The Kubernetes Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package podres

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"google.golang.org/grpc"

	podresourcesapi "github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres/api/v1"
)

type FakeClient struct {
	path string
}

func (fc *FakeClient) List(ctx context.Context, in *podresourcesapi.ListPodResourcesRequest, opts ...grpc.CallOption) (*podresourcesapi.ListPodResourcesResponse, error) {
	var resp podresourcesapi.ListPodResourcesResponse
	err := fc.readJSON("list.json", &resp)
	return &resp, err
}

func (fc *FakeClient) GetAllocatableResources(ctx context.Context, in *podresourcesapi.AllocatableResourcesRequest, opts ...grpc.CallOption) (*podresourcesapi.AllocatableResourcesResponse, error) {
	var resp podresourcesapi.AllocatableResourcesResponse
	err := fc.readJSON("get_allocatable_resources.json", &resp)
	return &resp, err
}

func (fc *FakeClient) Get(ctx context.Context, in *podresourcesapi.GetPodResourcesRequest, opts ...grpc.CallOption) (*podresourcesapi.GetPodResourcesResponse, error) {
	var resp podresourcesapi.GetPodResourcesResponse
	err := fc.readJSON("get.json", &resp)
	return &resp, err
}

func (fc *FakeClient) readJSON(name string, v any) error {
	data, err := os.ReadFile(filepath.Join(fc.path, name))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func NewFakePodResourcesLister(path string) podresourcesapi.PodResourcesListerClient {
	return &FakeClient{
		path: path,
	}
}
