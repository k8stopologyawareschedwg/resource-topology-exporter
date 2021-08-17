/*
Copyright 2020 The Kubernetes Authors.

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

package resourcemonitor

import (
	"context"
	"fmt"
	"log"

	"k8s.io/api/core/v1"
	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/prometheus"
)

type PodResourcesScanner struct {
	namespace         string
	podResourceClient podresourcesapi.PodResourcesListerClient
}

func NewPodResourcesScanner(namespace string, podResourceClient podresourcesapi.PodResourcesListerClient) (ResourcesScanner, error) {
	resourcemonitorInstance := &PodResourcesScanner{
		namespace:         namespace,
		podResourceClient: podResourceClient,
	}
	if resourcemonitorInstance.namespace != "" {
		log.Printf("watching namespace %q", resourcemonitorInstance.namespace)
	} else {
		log.Printf("watching all namespaces")
	}

	return resourcemonitorInstance, nil
}

// isWatchable tells if the the given namespace should be watched.
func (resMon *PodResourcesScanner) isWatchable(podNamespace string) bool {
	if resMon.namespace == "" {
		return true
	}
	//TODO:  add an explicit check for guaranteed pods
	return resMon.namespace == podNamespace
}

// Scan gathers all the PodResources from the system, using the podresources API client.
func (resMon *PodResourcesScanner) Scan() ([]PodResources, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultPodResourcesTimeout)
	defer cancel()

	//Pod Resource API client
	resp, err := resMon.podResourceClient.List(ctx, &podresourcesapi.ListPodResourcesRequest{})
	if err != nil {
		prometheus.UpdatePodResourceApiCallsFailureMetric("list")
		return nil, fmt.Errorf("can't receive response: %v.Get(_) = _, %v", resMon.podResourceClient, err)
	}

	var podResData []PodResources

	for _, podResource := range resp.GetPodResources() {
		if !resMon.isWatchable(podResource.GetNamespace()) {
			continue
		}

		podRes := PodResources{
			Name:      podResource.GetName(),
			Namespace: podResource.GetNamespace(),
		}

		for _, container := range podResource.GetContainers() {
			contRes := ContainerResources{
				Name: container.Name,
			}

			cpuIDs := container.GetCpuIds()
			if len(cpuIDs) > 0 {
				var resCPUs []string
				for _, cpuID := range container.GetCpuIds() {
					resCPUs = append(resCPUs, fmt.Sprintf("%d", cpuID))
				}
				contRes.Resources = []ResourceInfo{
					{
						Name: v1.ResourceCPU,
						Data: resCPUs,
					},
				}
			}

			for _, device := range container.GetDevices() {
				topology := getTopology(device.GetTopology())
				contRes.Resources = append(contRes.Resources, ResourceInfo{
					Name:     v1.ResourceName(device.ResourceName),
					Data:     device.DeviceIds,
					Topology: topology,
				})
			}

			for _, block := range container.GetMemory() {
				if block.GetSize_() == 0 {
					continue
				}

				topology := getTopology(block.GetTopology())
				contRes.Resources = append(contRes.Resources, ResourceInfo{
					Name:     v1.ResourceName(block.MemoryType),
					Data:     []string{fmt.Sprintf("%d", block.GetSize_())},
					Topology: topology,
				})
			}

			if len(contRes.Resources) == 0 {
				continue
			}

			podRes.Containers = append(podRes.Containers, contRes)
		}

		if len(podRes.Containers) == 0 {
			continue
		}

		podResData = append(podResData, podRes)

	}

	return podResData, nil
}

func getTopology(topologyInfo *podresourcesapi.TopologyInfo) []int {
	if topologyInfo == nil {
		return nil
	}

	var topology []int
	for _, node := range topologyInfo.Nodes {
		if node == nil {
			continue
		}

		topology = append(topology, int(node.ID))
	}

	return topology
}
