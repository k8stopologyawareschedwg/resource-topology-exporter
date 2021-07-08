package resourcetopologyexporter

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/kubeconf"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/nrtupdater"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podrescli"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcemonitor"
)

const (
	StateCPUManager    string = "cpu_manager_state"
	StateMemoryManager string = "memory_manager_state"
	StateDeviceManager string = "kubelet_internal_checkpoint"
)

type Args struct {
	Debug              bool
	ReferenceContainer *podrescli.ContainerIdent
}

func ContainerIdentFromEnv() *podrescli.ContainerIdent {
	cntIdent := podrescli.ContainerIdent{
		Namespace:     os.Getenv("REFERENCE_NAMESPACE"),
		PodName:       os.Getenv("REFERENCE_POD_NAME"),
		ContainerName: os.Getenv("REFERENCE_CONTAINER_NAME"),
	}
	if cntIdent.Namespace == "" || cntIdent.PodName == "" || cntIdent.ContainerName == "" {
		return nil
	}
	return &cntIdent
}

func ContainerIdentFromString(ident string) (*podrescli.ContainerIdent, error) {
	if ident == "" {
		return nil, nil
	}
	items := strings.Split(ident, "/")
	if len(items) != 3 {
		return nil, fmt.Errorf("malformed ident: %q", ident)
	}
	cntIdent := &podrescli.ContainerIdent{
		Namespace:     strings.TrimSpace(items[0]),
		PodName:       strings.TrimSpace(items[1]),
		ContainerName: strings.TrimSpace(items[2]),
	}
	log.Printf("reference container: %s", cntIdent)
	return cntIdent, nil
}

func Execute(nrtupdaterArgs nrtupdater.Args, resourcemonitorArgs resourcemonitor.Args, rteArgs Args) error {
	klConfig, err := kubeconf.GetKubeletConfigFromLocalFile(resourcemonitorArgs.KubeletConfigFile)
	if err != nil {
		return fmt.Errorf("error getting topology Manager Policy: %w", err)
	}
	tmPolicy := klConfig.TopologyManagerPolicy
	log.Printf("detected kubelet Topology Manager policy %q", tmPolicy)

	resMon, err := NewResourceMonitor(resourcemonitorArgs, rteArgs)
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

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create the watcher: %w", err)
	}
	defer watcher.Close()

	for _, stateDir := range resourcemonitorArgs.KubeletStateDirs {
		log.Printf("kubelet state dir: [%s]", stateDir)
		if stateDir == "" {
			continue
		}
		err := watcher.Add(stateDir)
		if err != nil {
			log.Printf("error adding watch on [%s]: %v", stateDir, err)
		} else {
			log.Printf("added watch on [%s]", stateDir)
		}
	}

	ticker := time.NewTicker(resourcemonitorArgs.SleepInterval)
	for {
		// TODO: what about closed channels?
		select {
		case <-ticker.C:
			eventsChan <- struct{}{}
			log.Printf("timer update trigger")

		case event := <-watcher.Events:
			log.Printf("fsnotify event from %q: %v", event.Name, event.Op)
			if IsTriggeringFSNotifyEvent(event) {
				eventsChan <- struct{}{}
				log.Printf("fsnotify update trigger")
			}

		case err := <-watcher.Errors:
			// and yes, keep going
			log.Printf("fsnotify error: %v", err)
		}
	}

	return nil // unreachable
}

func IsTriggeringFSNotifyEvent(event fsnotify.Event) bool {
	filename := filepath.Base(event.Name)
	if filename != StateCPUManager &&
		filename != StateMemoryManager &&
		filename != StateDeviceManager {
		return false
	}
	// turns out rename is reported as
	// 1. RENAME (old) <- unpredictable
	// 2. CREATE (new) <- we trigger here
	// admittedly we can get some false positives, but that
	// is expected to be not that big of a deal.
	return (event.Op & fsnotify.Create) == fsnotify.Create
}

type ResourceMonitor struct {
	resScan resourcemonitor.ResourcesScanner
	resAggr resourcemonitor.ResourcesAggregator
}

func NewResourceMonitor(args resourcemonitor.Args, rteArgs Args) (*ResourceMonitor, error) {
	podResClient, err := podrescli.NewClient(args.PodResourceSocketPath, rteArgs.Debug, rteArgs.ReferenceContainer)
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
