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
	rnd      *rand.Rand
}

func NewGenerator(interval time.Duration, randSeed int64) *Generator {
	gen := Generator{
		infoChan: make(chan nrtupdater.MonitorInfo),
		interval: interval,
	}
	gen.Infos = gen.infoChan
	gen.rnd = rand.New(rand.NewSource(randSeed))
	return &gen
}

func (ge *Generator) Run() {
	ticker := time.NewTicker(ge.interval)

	// TODO: move to for { select {} } when we have more sources
	for range ticker.C {
		mi := nrtupdater.MonitorInfo{
			Timer: true,
			Zones: ge.MakeZones(),
		}
		ge.infoChan <- mi
		klog.V(5).Infof("generated periodic update: %v", dump.Object(mi.Zones))
	}
}

func (ge *Generator) MakeZones() v1alpha2.ZoneList {
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
				ge.MakeResourceInfoCPUs(128, 126, 126),
				ge.MakeResourceInfoDevices(16),
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
				ge.MakeResourceInfoCPUs(128, 126, 126),
				ge.MakeResourceInfoDevices(16),
			},
		},
	}
	return zones
}

func (ge *Generator) MakeResourceInfoDevices(count int) v1alpha2.ResourceInfo {
	return v1alpha2.ResourceInfo{
		Name:        "vendor.com/device",
		Capacity:    *resource.NewQuantity(16, resource.DecimalSI),
		Allocatable: *resource.NewQuantity(16, resource.DecimalSI),
		Available:   *resource.NewQuantity(int64(ge.rnd.Intn(17)), resource.DecimalSI),
	}
}

func (ge *Generator) MakeResourceInfoCPUs(capacity, allocatable, available int) v1alpha2.ResourceInfo {
	return v1alpha2.ResourceInfo{
		Name:        "cpu",
		Capacity:    *resource.NewQuantity(int64(capacity), resource.DecimalSI),
		Allocatable: *resource.NewQuantity(int64(allocatable), resource.DecimalSI),
		Available:   *resource.NewQuantity(int64(ge.rnd.Intn(available+1)), resource.DecimalSI),
	}
}
