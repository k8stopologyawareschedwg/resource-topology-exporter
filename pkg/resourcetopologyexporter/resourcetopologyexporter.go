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

	podResClient, err := podres.GetPodResClient(resourcemonitorArgs.PodResourceSocketPath)
	if err != nil {
		return fmt.Errorf("failed to get podresources client: %w", err)
	}
	var resScan resourcemonitor.ResourcesScanner

	resScan, err = resourcemonitor.NewPodResourcesScanner(resourcemonitorArgs.Namespace, podResClient)
	if err != nil {
		return fmt.Errorf("failed to initialize ResourceMonitor instance: %w", err)
	}

	// CAUTION: these resources are expected to change rarely - if ever.
	//So we are intentionally do this once during the process lifecycle.
	//TODO: Obtain node resources dynamically from the podresource API
	zonesChannel := make(chan v1alpha1.ZoneList)
	var zones v1alpha1.ZoneList

	resAggr, err := resourcemonitor.NewResourcesAggregator(resourcemonitorArgs.SysfsRoot, podResClient)
	if err != nil {
		return fmt.Errorf("failed to obtain node resource information: %w", err)
	}

	go func() {
		for {
			podResources, err := resScan.Scan()
			if err != nil {
				log.Printf("failed to scan pod resources: %v\n", err)
				continue
			}

			zones = resAggr.Aggregate(podResources)
			zonesChannel <- zones

			time.Sleep(resourcemonitorArgs.SleepInterval)
		}
	}()

	// Get new TopologyUpdater instance
	instance, err := nrtupdater.NewNRTUpdater(nrtupdaterArgs, tmPolicy)
	if err != nil {
		return fmt.Errorf("failed to initialize NRT updater: %w", err)
	}
	for {

		zonesValue := <-zonesChannel
		if err = instance.Update(zonesValue); err != nil {
			return fmt.Errorf("failed to update: %w", err)
		}
		if nrtupdaterArgs.Oneshot {
			break
		}
	}

	return nil // unreachable
}
