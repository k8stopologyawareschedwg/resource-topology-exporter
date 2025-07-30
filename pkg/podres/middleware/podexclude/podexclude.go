/*
 * Copyright 2022 Red Hat, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package podexclude

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"google.golang.org/grpc"

	"k8s.io/klog/v2"
	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"
)

type Item struct {
	NamespacePattern string `json:"namespacePattern"`
	NamePattern      string `json:"namePattern"`
}

type List []Item

func (items List) Clone() List {
	return append([]Item{}, items...)
}

func (items List) String() string {
	if len(items) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, item := range items {
		fmt.Fprintf(&sb, ", %s/%s", item.NamespacePattern, item.NamePattern)
	}
	return sb.String()[2:]
}

type filteringClient struct {
	debug bool
	cli   podresourcesapi.PodResourcesListerClient
	// namespace glob -> name glob
	podExcludes List
}

func (fc *filteringClient) FilterListResponse(resp *podresourcesapi.ListPodResourcesResponse) *podresourcesapi.ListPodResourcesResponse {
	retResp := podresourcesapi.ListPodResourcesResponse{
		PodResources: make([]*podresourcesapi.PodResources, 0, len(resp.GetPodResources())),
	}
	for _, podRes := range resp.GetPodResources() {
		if ShouldExclude(fc.podExcludes, podRes.GetNamespace(), podRes.GetName(), fc.debug) {
			continue
		}
		retResp.PodResources = append(retResp.PodResources, podRes)
	}
	return &retResp
}

func (fc *filteringClient) FilterAllocatableResponse(resp *podresourcesapi.AllocatableResourcesResponse) *podresourcesapi.AllocatableResourcesResponse {
	return resp // nothing to do here
}

func (fc *filteringClient) FilterGetResponse(resp *podresourcesapi.GetPodResourcesResponse) *podresourcesapi.GetPodResourcesResponse {
	return resp // TODO: not needed, but implement actual filtering for consistency
}

func (fc *filteringClient) List(ctx context.Context, in *podresourcesapi.ListPodResourcesRequest, opts ...grpc.CallOption) (*podresourcesapi.ListPodResourcesResponse, error) {
	resp, err := fc.cli.List(ctx, in, opts...)
	if err != nil {
		return resp, err
	}
	return fc.FilterListResponse(resp), nil
}

func (fc *filteringClient) GetAllocatableResources(ctx context.Context, in *podresourcesapi.AllocatableResourcesRequest, opts ...grpc.CallOption) (*podresourcesapi.AllocatableResourcesResponse, error) {
	resp, err := fc.cli.GetAllocatableResources(ctx, in, opts...)
	if err != nil {
		return resp, err
	}
	return fc.FilterAllocatableResponse(resp), nil
}

func (fc *filteringClient) Get(ctx context.Context, in *podresourcesapi.GetPodResourcesRequest, opts ...grpc.CallOption) (*podresourcesapi.GetPodResourcesResponse, error) {
	resp, err := fc.cli.Get(ctx, in, opts...)
	if err != nil {
		return resp, err
	}
	return fc.FilterGetResponse(resp), nil
}

func NewFromLister(cli podresourcesapi.PodResourcesListerClient, debug bool, podExcludes List) podresourcesapi.PodResourcesListerClient {
	klog.V(2).Infof("podexclude: ignoring: [%s]", podExcludes.String())
	return &filteringClient{
		debug:       debug,
		cli:         cli,
		podExcludes: podExcludes,
	}
}

func ShouldExclude(podExcludes List, namespace, name string, debug bool) bool {
	for _, item := range podExcludes {
		nsMatch, err := filepath.Match(item.NamespacePattern, namespace)
		if err != nil && debug {
			klog.Warningf("match error: namespace glob=%q pod=%s/%s: %v", item.NamespacePattern, namespace, name, err)
			continue
		}
		if !nsMatch {
			continue
		}
		nMatch, err := filepath.Match(item.NamePattern, name)
		if err != nil && debug {
			klog.Warningf("match error: name glob=%q pod=%s/%s: %v", item.NamePattern, namespace, name, err)
			continue
		}
		if !nMatch {
			continue
		}
		return true
	}
	return false
}
