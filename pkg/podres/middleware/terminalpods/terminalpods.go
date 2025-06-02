/*
Copyright 2023 The Kubernetes Authors.

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

package terminalpods

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	informerscorve1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	podresourcesapi "github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres/api/v1"
)

const terminalFieldSelector = "status.phase==Failed,status.phase==Succeeded"

type filteringClient struct {
	debug    bool
	cli      podresourcesapi.PodResourcesListerClient
	kcli     kubernetes.Interface
	informer informerscorve1.PodInformer
}

func (fc *filteringClient) List(ctx context.Context, in *podresourcesapi.ListPodResourcesRequest, opts ...grpc.CallOption) (*podresourcesapi.ListPodResourcesResponse, error) {
	resp, err := fc.cli.List(ctx, in, opts...)
	if err != nil {
		return resp, err
	}

	pods, err := fc.informer.Lister().List(labels.Everything())
	if err != nil {
		return resp, err
	}

	FilterFrom(resp, pods)
	return resp, nil
}

func (fc *filteringClient) GetAllocatableResources(ctx context.Context, in *podresourcesapi.AllocatableResourcesRequest, opts ...grpc.CallOption) (*podresourcesapi.AllocatableResourcesResponse, error) {
	return fc.cli.GetAllocatableResources(ctx, in, opts...)
}

func (fc *filteringClient) Get(ctx context.Context, in *podresourcesapi.GetPodResourcesRequest, opts ...grpc.CallOption) (*podresourcesapi.GetPodResourcesResponse, error) {
	return fc.cli.Get(ctx, in, opts...) // TODO: not needed, but we should implement filtering for consistency
}

func NewFromLister(ctx context.Context, cli podresourcesapi.PodResourcesListerClient, kcli kubernetes.Interface, resyncPeriod time.Duration, debug bool) (podresourcesapi.PodResourcesListerClient, error) {
	tweakFunc := func(opts *metav1.ListOptions) {
		// A pod is in a terminal state if .status.phase in (Failed, Succeeded) is true.
		opts.FieldSelector = terminalFieldSelector
	}
	factory := informers.NewSharedInformerFactoryWithOptions(kcli, resyncPeriod, informers.WithTweakListOptions(tweakFunc))
	podInformer := factory.Core().V1().Pods()
	factory.Start(ctx.Done())
	synced := factory.WaitForCacheSync(ctx.Done())
	for v, ok := range synced {
		if !ok {
			return nil, fmt.Errorf("caches failed to sync: %v", v)
		}
	}
	return &filteringClient{
		debug:    debug,
		cli:      cli,
		kcli:     kcli,
		informer: podInformer,
	}, nil
}

func FilterFrom(resp *podresourcesapi.ListPodResourcesResponse, pods []*corev1.Pod) {
	var filterResp []*podresourcesapi.PodResources
	podres := resp.GetPodResources()
	for i := 0; i < len(podres); i++ {
		pr := podres[i]
		found := false
		for _, pod := range pods {
			if pod.Namespace == pr.Namespace && pod.Name == pr.Name {
				found = true
				break
			}
		}
		if !found {
			filterResp = append(filterResp, pr)
		} else {
			klog.V(5).Infof("pod %s/%s is in terminal state, filtered from ListPodResourcesResponse", pr.Namespace, pr.Name)
		}
	}
	resp.PodResources = filterResp
}
