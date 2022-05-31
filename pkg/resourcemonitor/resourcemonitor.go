/*
Copyright 2021 The Kubernetes Authors.

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
	"os"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"

	"github.com/jaypipes/ghw"
	"github.com/k8stopologyawareschedwg/podfingerprint"

	topologyv1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/prometheus"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/sysinfo"
)

const (
	defaultPodResourcesTimeout = 10 * time.Second
	// obtained these values from node e2e tests : https://github.com/kubernetes/kubernetes/blob/82baa26905c94398a0d19e1b1ecf54eb8acb6029/test/e2e_node/util.go#L70
)

type ResourceExcludeList struct {
	ExcludeList map[string][]string
}

type Args struct {
	Namespace            string
	SysfsRoot            string
	ExcludeList          ResourceExcludeList
	RefreshNodeResources bool
	PodSetFingerprint    bool
	ExposeTiming         bool
}

type ResourceMonitor interface {
	Scan(excludeList ResourceExcludeList) (topologyv1alpha1.ZoneList, map[string]string, error)
}

// ToMapSet keeps the original keys, but replaces values with set.String types
func (r *ResourceExcludeList) ToMapSet() map[string]sets.String {
	asSet := make(map[string]sets.String)
	for k, v := range r.ExcludeList {
		asSet[k] = sets.NewString(v...)
	}
	return asSet
}

func (r *ResourceExcludeList) String() string {
	var b strings.Builder
	for name, items := range r.ExcludeList {
		fmt.Fprintf(&b, "- %s: [%s]\n", name, strings.Join(items, ", "))
	}
	return b.String()
}

// mapping resource -> count
type resourceCounter map[v1.ResourceName]int64

//mapping numa cell -> resource counter
type perNUMAResourceCounter map[int]resourceCounter

type resourceMonitor struct {
	nodeName          string
	args              Args
	podResCli         podresourcesapi.PodResourcesListerClient
	topo              *ghw.TopologyInfo
	coreIDToNodeIDMap map[int]int
	nodeCapacity      perNUMAResourceCounter
	nodeAllocatable   perNUMAResourceCounter
}

func NewResourceMonitor(podResCli podresourcesapi.PodResourcesListerClient, args Args) (*resourceMonitor, error) {
	topo, err := ghw.Topology(ghw.WithPathOverrides(ghw.PathOverrides{
		"/sys": args.SysfsRoot,
	}))
	if err != nil {
		return nil, err
	}
	nodeName := os.Getenv("NODE_NAME")
	return NewResourceMonitorWithTopology(nodeName, topo, podResCli, args)
}

func NewResourceMonitorWithTopology(nodeName string, topo *ghw.TopologyInfo, podResCli podresourcesapi.PodResourcesListerClient, args Args) (*resourceMonitor, error) {
	rm := &resourceMonitor{
		nodeName:          nodeName,
		podResCli:         podResCli,
		args:              args,
		topo:              topo,
		coreIDToNodeIDMap: MakeCoreIDToNodeIDMap(topo),
	}
	if !rm.args.RefreshNodeResources {
		klog.Infof("getting node resources once")
		if err := rm.updateNodeCapacity(); err != nil {
			return nil, err
		}
		if err := rm.updateNodeAllocatable(); err != nil {
			return nil, err
		}
	} else {
		klog.Infof("getting allocatable resources before each poll")
	}

	if rm.args.Namespace != "" {
		klog.Infof("watching namespace %q", rm.args.Namespace)
	} else {
		klog.Infof("watching all namespaces")
	}
	return rm, nil
}

func (rm *resourceMonitor) Scan(excludeList ResourceExcludeList) (topologyv1alpha1.ZoneList, map[string]string, error) {
	if rm.args.RefreshNodeResources {
		if err := rm.updateNodeCapacity(); err != nil {
			return nil, nil, err
		}
		if err := rm.updateNodeAllocatable(); err != nil {
			return nil, nil, err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultPodResourcesTimeout)
	defer cancel()
	resp, err := rm.podResCli.List(ctx, &podresourcesapi.ListPodResourcesRequest{})
	if err != nil {
		prometheus.UpdatePodResourceApiCallsFailureMetric("list")
		return nil, nil, err
	}

	respPodRes := resp.GetPodResources()
	allDevs := GetAllContainerDevices(respPodRes, rm.args.Namespace, rm.coreIDToNodeIDMap)
	allocated := ContainerDevicesToPerNUMAResourceCounters(allDevs)
	annotations := rm.annotationForResponse(respPodRes)

	excludeSet := excludeList.ToMapSet()
	zones := make(topologyv1alpha1.ZoneList, 0)
	// if there are no allocatable resources under a NUMA we might ended up with holes in the NRT objects.
	// this is why we're using the topology info and not the nodeAllocatable
	for nodeID := range rm.topo.Nodes {
		zone := topologyv1alpha1.Zone{
			Name:      makeZoneName(nodeID),
			Type:      "Node",
			Resources: make(topologyv1alpha1.ResourceInfoList, 0),
		}

		costs, err := makeCostsPerNumaNode(rm.topo.Nodes, nodeID)
		if err != nil {
			klog.Warningf("cannot find costs for NUMA node %d: %v", nodeID, err)
		} else {
			zone.Costs = costs
		}

		resCapCounters, ok := rm.nodeCapacity[nodeID]
		if !ok {
			resCapCounters = make(resourceCounter)
		}
		// the case of zero-value is handled below

		// check if NUMA has some allocatable resources
		resCounters, ok := rm.nodeAllocatable[nodeID]
		if !ok {
			// NUMA node doesn't have any allocatable resources. This means the returned counters map is empty.
			// Yet, the node exists in the topology, thus we consider all its CPUs are reserved
			resCounters = make(resourceCounter)
			resCounters[v1.ResourceCPU] = 0
		}

		for resName, resAlloc := range resCounters {
			if inExcludeSet(excludeSet, resName, rm.nodeName) {
				continue
			}

			resCapacity, ok := resCapCounters[resName]
			if !ok || resCapacity == 0 {
				klog.Warningf("zero capacity for resource %q on NUMA cell %d", resName, nodeID)
				resCapacity = resAlloc
			}
			if resAlloc > resCapacity {
				klog.Warningf("allocated more than capacity for %q on zone %q", resName.String(), zone.Name)
				// we trust more kubelet than ourselves atm.
				resCapacity = resAlloc
			}

			resUsed := allocated[nodeID][resName]

			resAvail := resAlloc - resUsed
			if resAvail < 0 {
				klog.Warningf("negative size for %q on zone %q", resName.String(), zone.Name)
				resAvail = 0
			}

			zone.Resources = append(zone.Resources, topologyv1alpha1.ResourceInfo{
				Name:        resName.String(),
				Available:   *resource.NewQuantity(resAvail, resource.DecimalSI),
				Allocatable: *resource.NewQuantity(resAlloc, resource.DecimalSI),
				Capacity:    *resource.NewQuantity(resCapacity, resource.DecimalSI),
			})
		}

		zones = append(zones, zone)
	}
	return zones, annotations, nil
}

func (rm *resourceMonitor) annotationForResponse(podRes []*podresourcesapi.PodResources) map[string]string {
	annotations := make(map[string]string)
	if rm.args.PodSetFingerprint {
		annotations[podfingerprint.Annotation] = ComputePodFingerprint(podRes)
	}
	return annotations
}

func (rm *resourceMonitor) updateNodeCapacity() error {
	memCounters, err := sysinfo.GetMemoryResourceCounters(sysinfo.Handle{})
	if err != nil {
		return err
	}

	hp2Mi := sysinfo.HugepageResourceNameFromSize(sysinfo.HugepageSize2Mi)
	hp1Gi := sysinfo.HugepageResourceNameFromSize(sysinfo.HugepageSize1Gi)

	// we care only about reservable resources, thus:
	// cpu, memory, hugepages
	perNUMARc := make(perNUMAResourceCounter)
	for nodeID := range rm.topo.Nodes {
		perNUMARc[nodeID] = resourceCounter{
			v1.ResourceCPU:         cpuCapacity(rm.topo, nodeID),
			v1.ResourceMemory:      memCounters[string(v1.ResourceMemory)][nodeID],
			v1.ResourceName(hp2Mi): memCounters[hp2Mi][nodeID],
			v1.ResourceName(hp1Gi): memCounters[hp1Gi][nodeID],
		}
	}
	rm.nodeCapacity = perNUMARc
	return nil
}

func (rm *resourceMonitor) updateNodeAllocatable() error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultPodResourcesTimeout)
	defer cancel()
	allocRes, err := rm.podResCli.GetAllocatableResources(ctx, &podresourcesapi.AllocatableResourcesRequest{})
	if err != nil {
		prometheus.UpdatePodResourceApiCallsFailureMetric("get_allocatable_resources")
		return err
	}

	allDevs := NormalizeContainerDevices(allocRes.GetDevices(), allocRes.GetMemory(), allocRes.GetCpuIds(), rm.coreIDToNodeIDMap)
	rm.nodeAllocatable = ContainerDevicesToPerNUMAResourceCounters(allDevs)
	return nil
}

func GetAllContainerDevices(podRes []*podresourcesapi.PodResources, namespace string, coreIDToNodeIDMap map[int]int) []*podresourcesapi.ContainerDevices {
	allCntRes := []*podresourcesapi.ContainerDevices{}
	for _, pr := range podRes {
		// filter by namespace (if given)
		if namespace != "" && namespace != pr.GetNamespace() {
			continue
		}
		for _, cnt := range pr.GetContainers() {
			allCntRes = append(allCntRes, NormalizeContainerDevices(cnt.GetDevices(), cnt.GetMemory(), cnt.GetCpuIds(), coreIDToNodeIDMap)...)
		}

	}
	return allCntRes
}

func ComputePodFingerprint(podRes []*podresourcesapi.PodResources) string {
	fp := podfingerprint.NewFingerprint(len(podRes))
	for _, pr := range podRes {
		fp.AddPod(pr)
	}
	return fp.Sign()
}

func NormalizeContainerDevices(devices []*podresourcesapi.ContainerDevices, memoryBlocks []*podresourcesapi.ContainerMemory, cpuIds []int64, coreIDToNodeIDMap map[int]int) []*podresourcesapi.ContainerDevices {
	contDevs := append([]*podresourcesapi.ContainerDevices{}, devices...)

	cpusPerNuma := make(map[int][]string)
	for _, cpuID := range cpuIds {
		nodeID, ok := coreIDToNodeIDMap[int(cpuID)]
		if !ok {
			klog.Warningf("cannot find the NUMA node for CPU %d", cpuID)
			continue
		}
		cpusPerNuma[nodeID] = append(cpusPerNuma[nodeID], fmt.Sprintf("%d", cpuID))
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

	for _, block := range memoryBlocks {
		blockSize := block.GetSize_()
		if blockSize == 0 {
			continue
		}

		for _, node := range block.GetTopology().GetNodes() {
			contDevs = append(contDevs, &podresourcesapi.ContainerDevices{
				ResourceName: block.MemoryType,
				DeviceIds:    []string{fmt.Sprintf("%d", blockSize)},
				Topology: &podresourcesapi.TopologyInfo{
					Nodes: []*podresourcesapi.NUMANode{
						{ID: int64(node.ID)},
					},
				},
			})
		}
	}

	return contDevs
}

func ContainerDevicesToPerNUMAResourceCounters(devices []*podresourcesapi.ContainerDevices) perNUMAResourceCounter {
	perNUMARc := make(perNUMAResourceCounter)
	for _, device := range devices {
		resourceName := device.GetResourceName()
		for _, node := range device.GetTopology().GetNodes() {
			nodeID := int(node.GetID())
			nodeRes, ok := perNUMARc[nodeID]
			if !ok {
				nodeRes = make(resourceCounter)
			}
			if resourceName == string(v1.ResourceMemory) || strings.HasPrefix(resourceName, v1.ResourceHugePagesPrefix) {
				var memSize int64
				for _, devBlock := range device.GetDeviceIds() {
					// can't fail, we constructed in a correct way
					devBlockSize, _ := strconv.ParseInt(devBlock, 10, 64)
					memSize += devBlockSize
				}
				nodeRes[v1.ResourceName(resourceName)] += memSize
			} else {
				nodeRes[v1.ResourceName(resourceName)] += int64(len(device.GetDeviceIds()))
			}
			perNUMARc[nodeID] = nodeRes
		}
	}
	return perNUMARc
}

func MakeCoreIDToNodeIDMap(topo *ghw.TopologyInfo) map[int]int {
	coreToNode := make(map[int]int)
	for _, node := range topo.Nodes {
		for _, core := range node.Cores {
			for _, procID := range core.LogicalProcessors {
				coreToNode[procID] = node.ID
			}
		}
	}
	return coreToNode
}

// makeCostsPerNumaNode builds the cost map to reach all the known NUMA zones (mapping (numa zone) -> cost) starting from the given NUMA zone.
func makeCostsPerNumaNode(nodes []*ghw.TopologyNode, nodeIDSrc int) ([]topologyv1alpha1.CostInfo, error) {
	nodeSrc := findNodeByID(nodes, nodeIDSrc)
	if nodeSrc == nil {
		return nil, fmt.Errorf("unknown node: %d", nodeIDSrc)
	}
	nodeCosts := make([]topologyv1alpha1.CostInfo, 0, len(nodeSrc.Distances))
	for nodeIDDst, dist := range nodeSrc.Distances {
		// TODO: this assumes there are no holes (= no offline node) in the distance vector
		nodeCosts = append(nodeCosts, topologyv1alpha1.CostInfo{
			Name:  makeZoneName(nodeIDDst),
			Value: int64(dist),
		})
	}
	return nodeCosts, nil
}

// makeZoneName returns the canonical name of a NUMA zone from its ID.
func makeZoneName(nodeID int) string {
	return fmt.Sprintf("node-%d", nodeID)
}

func findNodeByID(nodes []*ghw.TopologyNode, nodeID int) *ghw.TopologyNode {
	for _, node := range nodes {
		if node.ID == nodeID {
			return node
		}
	}
	return nil
}

func inExcludeSet(excludeSet map[string]sets.String, resName v1.ResourceName, nodeName string) bool {
	if set, ok := excludeSet["*"]; ok && set.Has(string(resName)) {
		return true
	}
	if set, ok := excludeSet[nodeName]; ok && set.Has(string(resName)) {
		return true
	}
	return false
}

func cpuCapacity(topo *ghw.TopologyInfo, nodeID int) int64 {
	nodeSrc := findNodeByID(topo.Nodes, nodeID)
	logicalCoresPerNUMA := 0
	for _, core := range nodeSrc.Cores {
		logicalCoresPerNUMA += len(core.LogicalProcessors)
	}
	return int64(logicalCoresPerNUMA)
}
