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
	"os"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"

	"github.com/jaypipes/ghw"

	topologyv1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/prometheus"
)

const (
	defaultPodResourcesTimeout = 10 * time.Second
	// obtained these values from node e2e tests : https://github.com/kubernetes/kubernetes/blob/82baa26905c94398a0d19e1b1ecf54eb8acb6029/test/e2e_node/util.go#L70
)

type nodeResources struct {
	perNUMACapacity map[int]map[v1.ResourceName]int64
	// mapping: resourceName -> resourceID -> nodeID
	resourceID2NUMAID map[string]map[string]int
	topo              *ghw.TopologyInfo
}

type resourceData struct {
	allocatable int64
	capacity    int64
}

func NewResourcesAggregator(sysfsRoot string, podResourceClient podresourcesapi.PodResourcesListerClient) (ResourcesAggregator, error) {
	var err error

	topo, err := ghw.Topology(ghw.WithPathOverrides(ghw.PathOverrides{
		"/sys": sysfsRoot,
	}))
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultPodResourcesTimeout)
	defer cancel()

	//Pod Resource API client
	resp, err := podResourceClient.GetAllocatableResources(ctx, &podresourcesapi.AllocatableResourcesRequest{})
	if err != nil {
		prometheus.UpdatePodResourceApiCallsFailureMetric("get_allocatable_resources")
		return nil, fmt.Errorf("Can't receive response: %v.Get(_) = _, %v", podResourceClient, err)
	}

	return NewResourcesAggregatorFromData(topo, resp), nil
}

// NewResourcesAggregatorFromData is used to aggregate resource information based on the received data from underlying hardware and podresource API
func NewResourcesAggregatorFromData(topo *ghw.TopologyInfo, resp *podresourcesapi.AllocatableResourcesResponse) ResourcesAggregator {
	allDevs := GetContainerDevicesFromAllocatableResources(resp, topo)
	return &nodeResources{
		topo:              topo,
		resourceID2NUMAID: makeResourceMap(len(topo.Nodes), allDevs),
		perNUMACapacity:   MakeNodeCapacity(allDevs, resp.GetMemory()),
	}
}

// Aggregate provides the mapping (numa zone name) -> Zone from the given PodResources.
func (noderesourceData *nodeResources) Aggregate(podResData []PodResources, excludeList ResourceExcludeList) topologyv1alpha1.ZoneList {
	clusterNodeName := getNodeName()
	excludeListAsSet := excludeList.ToMapSet()
	perNuma := make(map[int]map[v1.ResourceName]*resourceData)
	for nodeID, nodeRes := range noderesourceData.perNUMACapacity {
		perNuma[nodeID] = make(map[v1.ResourceName]*resourceData)
		for resName, resCap := range nodeRes {
			if set, ok := excludeListAsSet["*"]; ok && set.Has(string(resName)) {
				continue
			}
			if set, ok := excludeListAsSet[clusterNodeName]; ok && set.Has(string(resName)) {
				continue
			}
			perNuma[nodeID][resName] = &resourceData{
				capacity:    resCap,
				allocatable: resCap,
			}
		}
	}

	for _, podRes := range podResData {
		for _, contRes := range podRes.Containers {
			for _, res := range contRes.Resources {
				if res.Name == v1.ResourceMemory || strings.HasPrefix(string(res.Name), v1.ResourceHugePagesPrefix) {
					noderesourceData.updateMemoryAllocatable(perNuma, res)
					continue
				}

				noderesourceData.updateAllocatable(perNuma, res)
			}
		}
	}

	// zones := make([]topologyv1alpha1.Zone, 0)
	zones := make(topologyv1alpha1.ZoneList, 0)
	for nodeID, resList := range perNuma {
		zone := topologyv1alpha1.Zone{
			Name:      makeZoneName(nodeID),
			Type:      "Node",
			Resources: make(topologyv1alpha1.ResourceInfoList, 0),
		}

		costs, err := makeCostsPerNumaNode(noderesourceData.topo.Nodes, nodeID)
		if err != nil {
			log.Printf("cannot find costs for NUMA node %d: %v", nodeID, err)
		} else {
			zone.Costs = costs
		}

		for name, resData := range resList {
			allocatableQty := *resource.NewQuantity(resData.allocatable, resource.DecimalSI)
			capacityQty := *resource.NewQuantity(resData.capacity, resource.DecimalSI)
			zone.Resources = append(zone.Resources, topologyv1alpha1.ResourceInfo{
				Name:        name.String(),
				Allocatable: intstr.FromString(allocatableQty.String()),
				Capacity:    intstr.FromString(capacityQty.String()),
			})
		}
		zones = append(zones, zone)
	}
	return zones
}

// GetContainerDevicesFromAllocatableResources normalize all compute resources to ContainerDevices.
// This is helpful because cpuIDs are not represented as ContainerDevices, but with a different format;
// Having a consistent representation of all the resources as ContainerDevices makes it simpler for
func GetContainerDevicesFromAllocatableResources(availRes *podresourcesapi.AllocatableResourcesResponse, topo *ghw.TopologyInfo) []*podresourcesapi.ContainerDevices {
	var contDevs []*podresourcesapi.ContainerDevices
	for _, dev := range availRes.GetDevices() {
		contDevs = append(contDevs, dev)
	}

	cpuIDToNodeIDMap := MakeLogicalCoreIDToNodeIDMap(topo)

	cpusPerNuma := make(map[int][]string)
	for _, cpuID := range availRes.GetCpuIds() {
		nodeID, ok := cpuIDToNodeIDMap[int(cpuID)]
		if !ok {
			log.Printf("cannot find the NUMA node for CPU %d", cpuID)
			continue
		}

		cpuIDList := cpusPerNuma[nodeID]
		cpuIDList = append(cpuIDList, fmt.Sprintf("%d", cpuID))
		cpusPerNuma[nodeID] = cpuIDList
	}

	for nodeID, cpuList := range cpusPerNuma {
		contDevs = append(contDevs, &podresourcesapi.ContainerDevices{
			ResourceName: string(v1.ResourceCPU),
			DeviceIds:    cpuList,
			Topology: &podresourcesapi.TopologyInfo{
				Nodes: []*podresourcesapi.NUMANode{
					{ID: int64(nodeID)},
				},
			},
		})
	}

	return contDevs
}

// updateAllocatable computes the actually alloctable resources.
// This function assumes the allocatable resources are initialized to be equal to the capacity.
func (noderesourceData *nodeResources) updateAllocatable(numaData map[int]map[v1.ResourceName]*resourceData, ri ResourceInfo) {
	for _, resID := range ri.Data {
		resName := string(ri.Name)
		resMap, ok := noderesourceData.resourceID2NUMAID[resName]
		if !ok {
			log.Printf("unknown resource %q", ri.Name)
			continue
		}
		nodeID, ok := resMap[resID]
		if !ok {
			log.Printf("unknown resource %q: %q", resName, resID)
			continue
		}
		numaData[nodeID][ri.Name].allocatable--
	}
}

// makeZoneName returns the canonical name of a NUMA zone from its ID.
func makeZoneName(nodeID int) string {
	return fmt.Sprintf("node-%d", nodeID)
}

// MakeNodeCapacity computes the node capacity as mapping (NUMA node ID) -> Resource -> Capacity (amount, int).
// The computation is done assuming all the resources to represent the capacity for are represented on a slice
// of ContainerDevices. No special treatment is done for CPU IDs. See GetContainerDevicesFromAllocatableResources.
func MakeNodeCapacity(devices []*podresourcesapi.ContainerDevices, memoryBlocks []*podresourcesapi.ContainerMemory) map[int]map[v1.ResourceName]int64 {
	perNUMACapacity := make(map[int]map[v1.ResourceName]int64)
	// initialize with the capacities
	for _, device := range devices {
		resourceName := device.GetResourceName()
		for _, node := range device.GetTopology().GetNodes() {
			nodeID := int(node.GetID())
			nodeRes, ok := perNUMACapacity[nodeID]
			if !ok {
				nodeRes = make(map[v1.ResourceName]int64)
			}
			nodeRes[v1.ResourceName(resourceName)] += int64(len(device.GetDeviceIds()))
			perNUMACapacity[nodeID] = nodeRes
		}
	}

	for _, block := range memoryBlocks {
		memoryType := v1.ResourceName(block.GetMemoryType())

		if block.GetTopology() == nil {
			continue
		}

		for _, node := range block.GetTopology().GetNodes() {
			nodeID := int(node.GetID())
			if _, ok := perNUMACapacity[nodeID]; !ok {
				perNUMACapacity[nodeID] = make(map[v1.ResourceName]int64)
			}

			if _, ok := perNUMACapacity[nodeID][memoryType]; !ok {
				perNUMACapacity[nodeID][memoryType] = 0
			}

			// I do not like the idea to cast from uint64 to int64, but until the memory size does not go over
			// 8589934592Gi, it should be ok
			perNUMACapacity[nodeID][memoryType] += int64(block.GetSize_())
		}
	}

	return perNUMACapacity
}

func MakeLogicalCoreIDToNodeIDMap(topo *ghw.TopologyInfo) map[int]int {
	core2node := make(map[int]int)
	for _, node := range topo.Nodes {
		for _, core := range node.Cores {
			for _, procID := range core.LogicalProcessors {
				core2node[procID] = node.ID
			}
		}
	}
	return core2node
}

// makeResourceMap creates the mapping (resource name) -> (device ID) -> (NUMA node ID) from the given slice of ContainerDevices.
// this is useful to quickly learn the NUMA ID of a given (resource, device).
func makeResourceMap(numaNodes int, devices []*podresourcesapi.ContainerDevices) map[string]map[string]int {
	resourceMap := make(map[string]map[string]int)

	for _, device := range devices {
		resourceName := device.GetResourceName()
		_, ok := resourceMap[resourceName]
		if !ok {
			resourceMap[resourceName] = make(map[string]int)
		}
		for _, node := range device.GetTopology().GetNodes() {
			nodeID := int(node.GetID())
			for _, deviceID := range device.GetDeviceIds() {
				resourceMap[resourceName][deviceID] = nodeID
			}
		}
	}
	return resourceMap
}

// makeCostsPerNumaNode builds the cost map to reach all the known NUMA zones (mapping (numa zone) -> cost) starting from the given NUMA zone.
func makeCostsPerNumaNode(nodes []*ghw.TopologyNode, nodeIDSrc int) ([]topologyv1alpha1.CostInfo, error) {
	nodeSrc := findNodeByID(nodes, nodeIDSrc)
	if nodeSrc == nil {
		return nil, fmt.Errorf("unknown node: %d", nodeIDSrc)
	}
	nodeCosts := make([]topologyv1alpha1.CostInfo, 0)
	for nodeIDDst, dist := range nodeSrc.Distances {
		// TODO: this assumes there are no holes (= no offline node) in the distance vector
		nodeCosts = append(nodeCosts, topologyv1alpha1.CostInfo{
			Name:  makeZoneName(nodeIDDst),
			Value: dist,
		})
	}
	return nodeCosts, nil
}

func findNodeByID(nodes []*ghw.TopologyNode, nodeID int) *ghw.TopologyNode {
	for _, node := range nodes {
		if node.ID == nodeID {
			return node
		}
	}
	return nil
}

func getNodeName() string {
	return os.Getenv("NODE_NAME")
}

// updateMemoryAllocatable computes the actual amount of the allocatable memory.
// This function assumes the allocatable resources are initialized to be equal to the capacity.
func (noderesourceData *nodeResources) updateMemoryAllocatable(numaData map[int]map[v1.ResourceName]*resourceData, ri ResourceInfo) {
	if len(ri.Topology) == 0 {
		klog.Warningf("failed to get the device %q NUMA nodes", string(ri.Name))
		return
	}

	if len(ri.Data) != 1 {
		klog.Warningf("failed to get the device %q size", string(ri.Name))
		return
	}

	requestedSize, _ := strconv.ParseInt(ri.Data[0], 10, 64)
	for _, numaNodeID := range ri.Topology {
		if requestedSize == 0 {
			return
		}

		if _, ok := numaData[numaNodeID]; !ok {
			return
		}

		if _, ok := numaData[numaNodeID][ri.Name]; !ok {
			return
		}

		if numaData[numaNodeID][ri.Name].allocatable == 0 {
			continue
		}

		if requestedSize >= numaData[numaNodeID][ri.Name].allocatable {
			requestedSize -= numaData[numaNodeID][ri.Name].allocatable
			numaData[numaNodeID][ri.Name].allocatable = 0
		} else {
			numaData[numaNodeID][ri.Name].allocatable -= requestedSize
			requestedSize = 0
		}
	}
}
