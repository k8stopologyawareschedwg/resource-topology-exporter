package resourcetopologyexporter

import (
	"fmt"
	"log"
	"time"

	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/kubeconf"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/nrtupdater"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcemonitor"
)

func Execute(nrtupdaterArgs nrtupdater.Args, resourcemonitorArgs resourcemonitor.Args) error {
	klConfig, err := kubeconf.GetKubeletConfigFromLocalFile(resourcemonitorArgs.KubeletConfigFile)
	if err != nil {
		return fmt.Errorf("error getting topology Manager Policy: %w", err)
	}
	tmPolicy := klConfig.TopologyManagerPolicy
	log.Printf("detected kubelet Topology Manager policy %q", tmPolicy)

	resMon, err := NewResourceMonitor(resourcemonitorArgs)
	if err != nil {
		return err
	}

	eventsChan := make(chan struct{})
	zonesChannel, _ := resMon.Run(eventsChan)

	upd, err := nrtupdater.NewNRTUpdater(nrtupdaterArgs, tmPolicy)
	if err != nil {
		return fmt.Errorf("failed to initialize NRT updater: %w", err)
	}
	upd.Run(zonesChannel)

	ticker := time.NewTicker(resourcemonitorArgs.SleepInterval)
	for {
		select {
		case <-ticker.C:
			eventsChan <- struct{}{}
		}
	}

	return nil // unreachable
}

type ResourceMonitor struct {
	resScan resourcemonitor.ResourcesScanner
	resAggr resourcemonitor.ResourcesAggregator
}

func NewResourceMonitor(args resourcemonitor.Args) (*ResourceMonitor, error) {
	podResClient, err := podres.GetPodResClient(args.PodResourceSocketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get podresources client: %w", err)
	}

	resScan, err := resourcemonitor.NewPodResourcesScanner(args.Namespace, podResClient)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize ResourceMonitor upd: %w", err)
	}
	// CAUTION: these resources are expected to change rarely - if ever.
	//So we are intentionally do this once during the process lifecycle.
	//TODO: Obtain node resources dynamically from the podresource API

	resAggr, err := resourcemonitor.NewResourcesAggregator(args.SysfsRoot, podResClient)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain node resource information: %w", err)
	}

	return &ResourceMonitor{
		resScan: resScan,
		resAggr: resAggr,
	}, nil
}

func (rm *ResourceMonitor) Run(eventsChan <-chan struct{}) (<-chan v1alpha1.ZoneList, chan<- struct{}) {
	zonesChannel := make(chan v1alpha1.ZoneList)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-eventsChan:
				tsBegin := time.Now()
				podResources, err := rm.resScan.Scan()
				if err != nil {
					log.Printf("failed to scan pod resources: %v\n", err)
					continue
				}

				zones := rm.resAggr.Aggregate(podResources)
				zonesChannel <- zones
				tsEnd := time.Now()

				log.Printf("read request received at %v completed in %v", tsBegin, tsEnd.Sub(tsBegin))
			case <-done:
				log.Printf("read stop at %v", time.Now())
				break
			}
		}
	}()
	return zonesChannel, done
}
