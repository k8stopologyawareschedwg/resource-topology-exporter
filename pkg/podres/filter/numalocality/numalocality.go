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

package numalocality

import (
	"fmt"

	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"

	podresfilter "github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres/filter"
)

const (
	CPU    string = "cpu"
	Memory string = "memory"
	Device string = "device"
)

func Verify(pr *podresourcesapi.PodResources) podresfilter.Result {
	if pr == nil {
		return podresfilter.Result{
			Allow: false,
		}
	}
	for _, cr := range pr.Containers {
		res := VerifyContainer(cr)
		if res.Allow {
			return res
		}
	}
	return podresfilter.Result{
		Allow: false,
	}
}

// AlwaysPass is deprecated; if needed use pkg/pkodres/filter.VerifyAlwaysPass
func AlwaysPass(_ *podresourcesapi.PodResources) bool {
	return true
}

// Required is deprecated: use Verify instead
func Required(pr *podresourcesapi.PodResources) bool {
	got := Verify(pr)
	return got.Allow
}

func GetNUMAID(topo *podresourcesapi.TopologyInfo) int {
	if topo == nil || topo.Nodes == nil {
		return -1
	}
	// if Nodes is not given, this means "don't care about locality". It's a legal representation.
	for _, node := range topo.Nodes {
		// setting node.ID == -1 is also a legal representation for "don't care about locality".
		if node.ID >= 0 {
			return int(node.ID)
		}
	}
	return -1
}

func VerifyContainer(cnt *podresourcesapi.ContainerResources) podresfilter.Result {
	if cnt == nil {
		return podresfilter.Result{
			Allow: false,
		}
	}

	// there's no correct order for checks here, or faster.
	// CPUs are the most frequent (because there's always here) exclusively
	// assigned devices, so we start from here.
	if len(cnt.CpuIds) > 0 {
		return podresfilter.Result{
			Allow:  true,
			Ident:  cnt.Name,
			Reason: CPU,
		}
	}
	for _, mem := range cnt.Memory {
		if GetNUMAID(mem.Topology) != -1 {
			return podresfilter.Result{
				Allow:  true,
				Ident:  cnt.Name,
				Reason: Memory,
			}
		}
	}
	for _, dev := range cnt.Devices {
		if len(dev.DeviceIds) > 0 && GetNUMAID(dev.Topology) != -1 {
			return podresfilter.Result{
				Allow:  true,
				Ident:  cnt.Name,
				Reason: Device,
			}
		}
	}

	return podresfilter.Result{
		Allow: false,
	}
}

// ResolveContainerPlacement finds the single NUMA node placement for a container;
// it returns the NUMA node ID if found, otherwise it returns -1.
// IMPORTANT: multiple-NUMA affinity is not supported (yet), thus this should be called only
// on single NUMA node topology manager policy.
func ResolveContainerPlacement(coreIDToNodeIDMap map[int]int, cnt *podresourcesapi.ContainerResources) (bool, int, error) {
	eligibleForPlacement := true
	if cnt == nil {
		return !eligibleForPlacement, -1, fmt.Errorf("nil container resources")
	}

	if len(cnt.CpuIds) > 0 {
		nodeID, ok := coreIDToNodeIDMap[int(cnt.CpuIds[0])]
		if !ok {
			//should never happen
			return eligibleForPlacement, -1, fmt.Errorf("CPU ID %d not found in coreIDToNodeIDMap", cnt.CpuIds[0])
		}
		return eligibleForPlacement, nodeID, nil
	}

	// TODO: handle multi-NUMAs for devices
	// we need to know if on multi-NUMA topology this data is available by
	// the pod-resources API or if it needs other means to get the NUMA node ID
	// https://redhat.atlassian.net/browse/CNF-23537
	// currently this assumes single NUMA node for all devices containers
	for _, dev := range cnt.Devices {
		nodeID := GetNUMAID(dev.Topology)
		if len(dev.DeviceIds) > 0 && nodeID != -1 {
			return eligibleForPlacement, nodeID, nil
		}
	}

	for _, mem := range cnt.Memory {
		nodeID := GetNUMAID(mem.Topology)
		if nodeID != -1 {
			return eligibleForPlacement, nodeID, nil
		}
	}

	return !eligibleForPlacement, -1, nil
}
