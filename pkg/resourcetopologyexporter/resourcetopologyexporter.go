package resourcetopologyexporter

import (
	"fmt"
	"time"

	"github.com/fsnotify/fsnotify"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"

	v1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/kubeconf"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/notification"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/nrtupdater"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podreadiness"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podrescli"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/prometheus"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcemonitor"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/topologypolicy"
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
	PodReadinessEnable     bool
	NotifyFilePath         string
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

	var condChan chan v1.PodCondition
	if rteArgs.PodReadinessEnable {
		condChan = make(chan v1.PodCondition)
		condIn, err := podreadiness.NewConditionInjector()
		if err != nil {
			return err
		}
		condIn.Run(condChan)
	}

	resObs, err := NewResourceObserver(cli, resourcemonitorArgs)
	if err != nil {
		return err
	}

	eventsChan := make(chan PollTrigger)
	infoChannel, _ := resObs.Run(eventsChan, condChan)

	upd, err := nrtupdater.NewNRTUpdater(nrtupdaterArgs, string(tmPolicy))
	if err != nil {
		return fmt.Errorf("failed to initialize NRT updater: %w", err)
	}
	upd.Run(infoChannel, condChan)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create the watcher: %w", err)
	}
	defer watcher.Close()

	filterFile, err := notification.AddFile(watcher, rteArgs.NotifyFilePath)
	if err != nil {
		return err
	}

	filterDirs, err := notification.AddDirs(watcher, rteArgs.KubeletStateDirs)
	if err != nil {
		return err
	}

	filterEvent := notification.MakeFilter(filterFile, filterDirs)

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
			if filterEvent(event) {
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

func getTopologyManagerPolicy(resourcemonitorArgs resourcemonitor.Args, rteArgs Args) (v1alpha1.TopologyManagerPolicy, error) {
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

type ResourceObserver struct {
	resMon      resourcemonitor.ResourceMonitor
	excludeList resourcemonitor.ResourceExcludeList
	condInj     *podreadiness.ConditionInjector
}

func NewResourceObserver(cli podresourcesapi.PodResourcesListerClient, args resourcemonitor.Args) (*ResourceObserver, error) {
	resMon, err := resourcemonitor.NewResourceMonitor(cli, args)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize ResourceMonitor: %w", err)
	}

	return &ResourceObserver{
		resMon:      resMon,
		excludeList: args.ExcludeList,
	}, nil
}

func (rm *ResourceObserver) Run(eventsChan <-chan PollTrigger, condChan chan<- v1.PodCondition) (<-chan nrtupdater.MonitorInfo, chan<- struct{}) {
	infoChannel := make(chan nrtupdater.MonitorInfo)
	done := make(chan struct{})
	var condStatus v1.ConditionStatus
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
					condStatus = v1.ConditionFalse
					continue
				}
				condStatus = v1.ConditionTrue
				infoChannel <- monInfo

				tsDiff := tsEnd.Sub(tsBegin)
				prometheus.UpdateOperationDelayMetric("podresources_scan", monInfo.UpdateReason(), float64(tsDiff.Milliseconds()))
				podreadiness.SetCondition(condChan, podreadiness.PodresourcesFetched, condStatus)
			case <-done:
				klog.Infof("read stop at %v", time.Now())
				break
			}
		}
	}()
	return infoChannel, done
}
