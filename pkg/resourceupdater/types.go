package resourceupdater

import (
	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha2"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/k8sannotations"
)

const (
	RTEUpdatePeriodic = "periodic"
	RTEUpdateReactive = "reactive"
)

// Command line arguments
type Args struct {
	NoPublish  bool   `json:"noPublish,omitempty"`
	Oneshot    bool   `json:"oneShot,omitempty"`
	Hostname   string `json:"hostname,omitempty"`
	KubeConfig string `json:"kubeConfig,omitempty"`
}

func (args Args) Clone() Args {
	return Args{
		NoPublish: args.NoPublish,
		Oneshot:   args.Oneshot,
		Hostname:  args.Hostname,
	}
}

type TMConfig struct {
	Policy string
	Scope  string
}

func (conf TMConfig) IsValid() bool {
	return conf.Policy != "" && conf.Scope != ""
}

func (conf TMConfig) ToAttributes() v1alpha2.AttributeList {
	return v1alpha2.AttributeList{
		{
			Name:  "topologyManagerScope",
			Value: conf.Scope,
		},
		{
			Name:  "topologyManagerPolicy",
			Value: conf.Policy,
		},
	}
}

type MonitorInfo struct {
	Timer       bool
	Zones       v1alpha2.ZoneList
	Attributes  v1alpha2.AttributeList
	Annotations map[string]string
}

func (mi MonitorInfo) UpdateReason() string {
	if mi.Timer {
		return RTEUpdatePeriodic
	}
	return RTEUpdateReactive
}

func (mi MonitorInfo) UpdateNRT(nrt *v1alpha2.NodeResourceTopology, tmConfig TMConfig) {
	nrt.Annotations = k8sannotations.Merge(nrt.Annotations, mi.Annotations)
	nrt.Annotations[k8sannotations.RTEUpdate] = mi.UpdateReason()
	nrt.Zones = mi.Zones.DeepCopy()
	nrt.Attributes = mi.Attributes.DeepCopy()
	nrt.Attributes = append(nrt.Attributes, tmConfig.ToAttributes()...)
	// TODO: check for duplicate attributes?
}
