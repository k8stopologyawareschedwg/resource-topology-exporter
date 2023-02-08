package fake

import (
	"math/rand"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog/v2"

	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha2"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/dump"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/nrtupdater"
)

type Generator struct {
	Infos    <-chan nrtupdater.MonitorInfo
	infoChan chan nrtupdater.MonitorInfo
	interval time.Duration
}

func NewGenerator(interval time.Duration) *Generator {
	gen := Generator{
		infoChan: make(chan nrtupdater.MonitorInfo),
		interval: interval,
	}
	gen.Infos = gen.infoChan
	return &gen
}

func (ge *Generator) Run() {
	ticker := time.NewTicker(ge.interval)

	for {
		select {
		case <-ticker.C:
			mi := nrtupdater.MonitorInfo{
				Timer: true,
				Zones: Zones(),
			}
			ge.infoChan <- mi
			klog.V(5).Infof("generated periodic update: %v", dump.Object(mi.Zones))
		}
	}
}

func Zones() v1alpha2.ZoneList {
	zones := v1alpha2.ZoneList{
		v1alpha2.Zone{
			Name: "fake-node-0",
			Type: "Node",
			Costs: []v1alpha2.CostInfo{
				{
					Name:  "fake-node-0",
					Value: 10,
				},
				{
					Name:  "fake-node-1",
					Value: 21,
				},
			},
			Resources: []v1alpha2.ResourceInfo{
				ResourceInfoCPUs(128, 126, 126),
				ResourceInfoDevices(16),
			},
		},
		v1alpha2.Zone{
			Name: "fake-node-1",
			Type: "Node",
			Costs: []v1alpha2.CostInfo{
				{
					Name:  "fake-node-1",
					Value: 10,
				},
				{
					Name:  "fake-node-0",
					Value: 21,
				},
			},
			Resources: []v1alpha2.ResourceInfo{
				ResourceInfoCPUs(128, 126, 126),
				ResourceInfoDevices(16),
			},
		},
	}
	return zones
}

func ResourceInfoDevices(count int) v1alpha2.ResourceInfo {
	return v1alpha2.ResourceInfo{
		Name:        "vendor.com/device",
		Capacity:    *resource.NewQuantity(16, resource.DecimalSI),
		Allocatable: *resource.NewQuantity(16, resource.DecimalSI),
		Available:   *resource.NewQuantity(int64(rand.Intn(17)), resource.DecimalSI),
	}
}

func ResourceInfoCPUs(capacity, allocatable, available int) v1alpha2.ResourceInfo {
	return v1alpha2.ResourceInfo{
		Name:        "cpu",
		Capacity:    *resource.NewQuantity(int64(capacity), resource.DecimalSI),
		Allocatable: *resource.NewQuantity(int64(allocatable), resource.DecimalSI),
		Available:   *resource.NewQuantity(int64(rand.Intn(available+1)), resource.DecimalSI),
	}
}
