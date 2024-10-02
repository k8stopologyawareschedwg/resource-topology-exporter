/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package proxy

import (
	"context"
	"net"

	"google.golang.org/grpc"

	criutil "k8s.io/cri-client/pkg/util"
	"k8s.io/klog/v2"
	podresourcesv1 "k8s.io/kubelet/pkg/apis/podresources/v1"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres"
)

// Proxy implements PodResourcesListerServer
type Proxy struct {
	cli      podresourcesv1.PodResourcesListerClient
	cleanup  podres.CleanupFunc
	lst      net.Listener
	server   *grpc.Server
	endpoint string
}

// NewV1PodResourcesServer returns a PodResourcesListerServer which lists pods provided by the PodsProvider
// with device information provided by the DevicesProvider
func New(cli podresourcesv1.PodResourcesListerClient, cleanup podres.CleanupFunc) *Proxy {
	return &Proxy{
		cli:     cli,
		cleanup: cleanup,
	}
}

func (p *Proxy) Cleanup() error {
	return p.cleanup()
}

func (p *Proxy) Setup(endpoint string) error {
	klog.V(4).InfoS("Preparing to serve the podresources proxy API", "endpoint", endpoint)
	defer klog.V(4).InfoS("Done preparing to serve the podresources proxy API", "endpoint", endpoint)

	p.server = grpc.NewServer()
	podresourcesv1.RegisterPodResourcesListerServer(p.server, p)

	klog.V(6).InfoS("Preparing listener", "endpoint", endpoint)
	lst, err := criutil.CreateListener(endpoint)
	if err != nil {
		return err
	}
	p.lst = lst
	p.endpoint = endpoint
	return nil
}

func (p *Proxy) Serve() error {
	klog.V(2).InfoS("Starting to serve the podresources proxy API", "endpoint", p.endpoint)
	defer klog.V(2).InfoS("Done serving the podresources proxy API", "endpoint", p.endpoint)
	return p.server.Serve(p.lst)
}

// List returns information about the resources assigned to pods on the node
func (p *Proxy) List(ctx context.Context, req *podresourcesv1.ListPodResourcesRequest) (*podresourcesv1.ListPodResourcesResponse, error) {
	return p.cli.List(ctx, req)
}

// GetAllocatableResources returns information about all the resources known by the server - this more like the capacity, not like the current amount of free resources.
func (p *Proxy) GetAllocatableResources(ctx context.Context, req *podresourcesv1.AllocatableResourcesRequest) (*podresourcesv1.AllocatableResourcesResponse, error) {
	return p.cli.GetAllocatableResources(ctx, req)
}

// Get returns information about the resources assigned to a specific pod
func (p *Proxy) Get(ctx context.Context, req *podresourcesv1.GetPodResourcesRequest) (*podresourcesv1.GetPodResourcesResponse, error) {
	return p.cli.Get(ctx, req)
}
