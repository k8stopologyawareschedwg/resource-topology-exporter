package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
)

type TopologyManagerPolicy string

const (
	// Constants of type TopologyManagerPolicy represent policy of the worker
	// node's resource management component. It's TopologyManager in kubele.
	// SingleNUMANodeContainerLevel represent single-numa-node policy of
	// the TopologyManager
	SingleNUMANodeContainerLevel TopologyManagerPolicy = "SingleNUMANodeContainerLevel"
	// SingleNUMANodePodLevel enables pod level resource counting, this policy assumes
	// TopologyManager policy single-numa-node also was set on the node
	SingleNUMANodePodLevel TopologyManagerPolicy = "SingleNUMANodePodLevel"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NodeResourceTopology is a specification for a Foo resource
type NodeResourceTopology struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	TopologyPolicies []string `json:"topologyPolicies"`
	Zones            ZoneList `json:"zones"`
}

// Zone is the spec for a NodeResourceTopology resource
type Zone struct {
	Name       string           `json:"name"`
	Type       string           `json:"type"`
	Parent     string           `json:"parent,omitempty"`
	Costs      CostList         `json:"costs,omitempty"`
	Attributes AttributeList    `json:"attributes,omitempty"`
	Resources  ResourceInfoList `json:"resources,omitempty"`
}

type ZoneList []Zone

type ResourceInfo struct {
	Name        string             `json:"name"`
	Allocatable intstr.IntOrString `json:"allocatable"`
	Capacity    intstr.IntOrString `json:"capacity"`
}
type ResourceInfoList []ResourceInfo

type CostInfo struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}
type CostList []CostInfo

type AttributeInfo struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type AttributeList []AttributeInfo

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NodeResourceTopologyList is a list of NodeResourceTopology resources
type NodeResourceTopologyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []NodeResourceTopology `json:"items"`
}
