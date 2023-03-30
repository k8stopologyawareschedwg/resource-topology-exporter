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
	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"
)

func AlwaysPass(_ *podresourcesapi.PodResources) bool {
	return true
}

func Required(pr *podresourcesapi.PodResources) bool {
	if pr == nil {
		return false
	}
	for _, cr := range pr.Containers {
		// there's no correct order for checks here, or faster.
		// CPUs are the most frequent (because there's always here) exclusively
		// assigned devices, so we start from here.
		if len(cr.CpuIds) > 0 {
			// exclusive CPUs
			return true
		}
		for _, mem := range cr.Memory {
			if IsPresent(mem.Topology) {
				// exclusive memory
				return true
			}
		}
		for _, dev := range cr.Devices {
			if len(dev.DeviceIds) > 0 && IsPresent(dev.Topology) {
				// exclusive device
				return true
			}
		}
	}
	return false
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
