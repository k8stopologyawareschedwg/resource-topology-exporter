package resourcetopologyexporter

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"

	"k8s.io/klog/v2"
	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/kubeconf"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/nrtupdater"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podrescli"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/prometheus"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcemonitor"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/topologypolicy"
)

const (
	StateCPUManager    string = "cpu_manager_state"
	StateMemoryManager string = "memory_manager_state"
	StateDeviceManager string = "kubelet_internal_checkpoint"
)

type Args struct {
	Debug                  bool
	ReferenceContainer     *podrescli.ContainerIdent
	TopologyManagerPolicy  string
	TopologyManagerScope   string
	KubeletConfigFile      string
	KubeletStateDirs       []string
	PodResourcesSocketPath string
	SleepInterval          time.Duration
}

type PollTrigger struct {
	Timer     bool
	Timestamp time.Time
}

func Execute(cli podresourcesapi.PodResourcesListerClient, nrtupdaterArgs nrtupdater.Args, resourcemonitorArgs resourcemonitor.Args, rteArgs Args) error {
	tmPolicy, err := getTopologyManagerPolicy(resourcemonitorArgs, rteArgs)
	if err != nil {
		return err
	}

	resMon, err := NewResourceMonitor(cli, resourcemonitorArgs, rteArgs)
	if err != nil {
		return err
	}

	eventsChan := make(chan PollTrigger)
	infoChannel, _ := resMon.Run(eventsChan)

	upd, err := nrtupdater.NewNRTUpdater(nrtupdaterArgs, string(tmPolicy))
	if err != nil {
		return fmt.Errorf("failed to initialize NRT updater: %w", err)
	}
	upd.Run(infoChannel)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create the watcher: %w", err)
	}
	defer watcher.Close()

	for _, stateDir := range rteArgs.KubeletStateDirs {
		klog.Infof("kubelet state dir: [%s]", stateDir)
		if stateDir == "" {
			continue
		}
		err := watcher.Add(stateDir)
		if err != nil {
			klog.Infof("error adding watch on [%s]: %v", stateDir, err)
		} else {
			klog.Infof("added watch on [%s]", stateDir)
		}
	}

	eventsChan <- PollTrigger{Timestamp: time.Now()}
	klog.V(2).Infof("initial update trigger")

	ticker := time.NewTicker(rteArgs.SleepInterval)
	for {
		// TODO: what about closed channels?
		select {
		case tickTs := <-ticker.C:
			eventsChan <- PollTrigger{Timer: true, Timestamp: tickTs}
			klog.V(4).Infof("timer update trigger")

		case event := <-watcher.Events:
			klog.V(5).Infof("fsnotify event from %q: %v", event.Name, event.Op)
			if IsTriggeringFSNotifyEvent(event) {
				eventsChan <- PollTrigger{Timestamp: time.Now()}
				klog.V(4).Infof("fsnotify update trigger")
			}

		case err := <-watcher.Errors:
			// and yes, keep going
			klog.Warningf("fsnotify error: %v", err)
		}
	}

	return nil // unreachable
}

func getTopologyManagerPolicy(resourcemonitorArgs resourcemonitor.Args, rteArgs Args) (topologypolicy.TopologyManagerPolicy, error) {
	if rteArgs.TopologyManagerPolicy != "" && rteArgs.TopologyManagerScope != "" {
		klog.Infof("using given Topology Manager policy %q scope %q", rteArgs.TopologyManagerPolicy, rteArgs.TopologyManagerScope)
		return topologypolicy.DetectTopologyPolicy(rteArgs.TopologyManagerPolicy, rteArgs.TopologyManagerScope), nil
	}
	if rteArgs.KubeletConfigFile != "" {
		klConfig, err := kubeconf.GetKubeletConfigFromLocalFile(rteArgs.KubeletConfigFile)
		if err != nil {
			return "", fmt.Errorf("error getting topology Manager Policy: %w", err)
		}
		klog.Infof("detected kubelet Topology Manager policy %q scope %q", klConfig.TopologyManagerPolicy, klConfig.TopologyManagerScope)
		return topologypolicy.DetectTopologyPolicy(klConfig.TopologyManagerPolicy, klConfig.TopologyManagerScope), nil
	}
	return "", fmt.Errorf("cannot find the kubelet Topology Manager policy")
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
	resMon      resourcemonitor.ResourceMonitor
	excludeList resourcemonitor.ResourceExcludeList
}

func NewResourceMonitor(cli podresourcesapi.PodResourcesListerClient, args resourcemonitor.Args, rteArgs Args) (*ResourceMonitor, error) {
	resMon, err := resourcemonitor.NewResourceMonitor(cli, args)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize ResourceMonitor: %w", err)
	}

	return &ResourceMonitor{
		resMon:      resMon,
		excludeList: args.ExcludeList,
	}, nil
}

func (rm *ResourceMonitor) Run(eventsChan <-chan PollTrigger) (<-chan nrtupdater.MonitorInfo, chan<- struct{}) {
	infoChannel := make(chan nrtupdater.MonitorInfo)
	done := make(chan struct{})
	go func() {
		lastWakeup := time.Now()
		for {
			select {
			case pt := <-eventsChan:
				var err error
				monInfo := nrtupdater.MonitorInfo{Timer: pt.Timer}

				tsWakeupDiff := pt.Timestamp.Sub(lastWakeup)
				lastWakeup = pt.Timestamp
				prometheus.UpdateWakeupDelayMetric(monInfo.UpdateReason(), float64(tsWakeupDiff.Milliseconds()))

				tsBegin := time.Now()
				monInfo.Zones, err = rm.resMon.Scan(rm.excludeList)
				tsEnd := time.Now()

				if err != nil {
					klog.Warningf("failed to scan pod resources: %w\n", err)
					continue
				}
				infoChannel <- monInfo

				tsDiff := tsEnd.Sub(tsBegin)
				prometheus.UpdateOperationDelayMetric("podresources_scan", monInfo.UpdateReason(), float64(tsDiff.Milliseconds()))
			case <-done:
				klog.Infof("read stop at %v", time.Now())
				break
			}
		}
	}()
	return infoChannel, done
}
