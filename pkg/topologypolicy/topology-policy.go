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

package topologypolicy

import (
	"k8s.io/kubernetes/pkg/kubelet/apis/config"

	v1alpha2 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha2"
)

// DetectTopologyPolicy returns string type which present
// both Topology manager policy and scope
func DetectTopologyPolicy(policy string, scope string) v1alpha2.TopologyManagerPolicy {
	switch policy {
	case config.SingleNumaNodeTopologyManagerPolicy:
		if scope == config.PodTopologyManagerScope {
			return v1alpha2.SingleNUMANodePodLevel
		} else if scope == config.ContainerTopologyManagerScope {
			return v1alpha2.SingleNUMANodeContainerLevel
		} else {
			// default scope for single-numa-node
			return v1alpha2.SingleNUMANodeContainerLevel
		}
	case config.RestrictedTopologyManagerPolicy:
		return v1alpha2.Restricted
	case config.BestEffortTopologyManagerPolicy:
		return v1alpha2.BestEffort
	case config.NoneTopologyManagerPolicy:
		return v1alpha2.None
	default:
		return v1alpha2.None
	}
}
