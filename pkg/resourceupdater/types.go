package resourceupdater

import (
	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha2"
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
