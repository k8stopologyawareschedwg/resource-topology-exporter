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
		if IsPresent(mem.Topology) {
			return podresfilter.Result{
				Allow:  true,
				Ident:  cnt.Name,
				Reason: Memory,
			}
		}
	}
	for _, dev := range cnt.Devices {
		if len(dev.DeviceIds) > 0 && IsPresent(dev.Topology) {
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

// AlwaysPass is deprecated; if needed use pkg/pkodres/filter.VerifyAlwaysPass
func AlwaysPass(_ *podresourcesapi.PodResources) bool {
	return true
}

// Required is deprecated: use Verify instead
func Required(pr *podresourcesapi.PodResources) bool {
	got := Verify(pr)
	return got.Allow
}

func IsPresent(topo *podresourcesapi.TopologyInfo) bool {
	if topo == nil || topo.Nodes == nil {
		return false
	}
	// if Nodes is not given, this means "don't care about locality". It's a legal representation.
	for _, node := range topo.Nodes {
		// setting node.ID == -1 is also a legal representation for "don't care about locality".
		if node.ID >= 0 {
			return true
		}
	}
	return false
}

// FindContainerSingleNUMAPlacement finds the single NUMA node placement for a container;
// it returns the NUMA node ID if found, otherwise it returns -1.
// IMPORTANT:multiple-NUMA affinity is not supported (yet), thus this should be called only
// on single NUMA node topology manager policy.
func FindContainerSingleNUMAPlacement(coreIDToNodeIDMap map[int]int, cnt *podresourcesapi.ContainerResources) (int, error) {
	if cnt == nil {
		return -1, fmt.Errorf("nil container resources")
	}

	if len(cnt.CpuIds) > 0 {
		nodeID, ok := coreIDToNodeIDMap[int(cnt.CpuIds[0])]
		if !ok {
			return -1, fmt.Errorf("cannot find the NUMA node for CPU %d", cnt.CpuIds[0])
		}
		return nodeID, nil
	}

	for _, dev := range cnt.Devices {
		if len(dev.DeviceIds) > 0 && IsPresent(dev.Topology) {
			return int(dev.Topology.Nodes[0].ID), nil
		}
	}

	for _, mem := range cnt.Memory {
		if IsPresent(mem.Topology) {
			return int(mem.Topology.Nodes[0].ID), nil
		}
	}

	return -1, nil
}
