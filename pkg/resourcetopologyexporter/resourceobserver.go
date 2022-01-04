package resourcetopologyexporter

import (
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/notification"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/nrtupdater"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podreadiness"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/prometheus"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcemonitor"
)

type ResourceObserver struct {
	Infos       <-chan nrtupdater.MonitorInfo
	resMon      resourcemonitor.ResourceMonitor
	excludeList resourcemonitor.ResourceExcludeList
	infoChan    chan nrtupdater.MonitorInfo
	stopChan    chan struct{}
}

func NewResourceObserver(cli podresourcesapi.PodResourcesListerClient, args resourcemonitor.Args) (*ResourceObserver, error) {
	resMon, err := resourcemonitor.NewResourceMonitor(cli, args)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize ResourceMonitor: %w", err)
	}

	resObs := ResourceObserver{
		resMon:      resMon,
		excludeList: args.ExcludeList,
		stopChan:    make(chan struct{}),
		infoChan:    make(chan nrtupdater.MonitorInfo),
	}
	resObs.Infos = resObs.infoChan
	return &resObs, nil
}

func (rm *ResourceObserver) Stop() {
	rm.stopChan <- struct{}{}
}

func (rm *ResourceObserver) Run(eventsChan <-chan notification.Event, condChan chan<- v1.PodCondition) {
	lastWakeup := time.Now()
	for {
		select {
		case ev := <-eventsChan:
			var err error
			monInfo := nrtupdater.MonitorInfo{Timer: ev.Timer}

			tsWakeupDiff := ev.Timestamp.Sub(lastWakeup)
			lastWakeup = ev.Timestamp
			prometheus.UpdateWakeupDelayMetric(monInfo.UpdateReason(), float64(tsWakeupDiff.Milliseconds()))

			tsBegin := time.Now()
			monInfo.Zones, err = rm.resMon.Scan(rm.excludeList)
			tsEnd := time.Now()

			condStatus := v1.ConditionTrue
			if err != nil {
				klog.Warningf("failed to scan pod resources: %w\n", err)
				condStatus = v1.ConditionFalse
				podreadiness.SetCondition(condChan, podreadiness.PodresourcesFetched, condStatus)
				continue
			}
			rm.infoChan <- monInfo

			tsDiff := tsEnd.Sub(tsBegin)
			prometheus.UpdateOperationDelayMetric("podresources_scan", monInfo.UpdateReason(), float64(tsDiff.Milliseconds()))
			podreadiness.SetCondition(condChan, podreadiness.PodresourcesFetched, condStatus)
		case <-rm.stopChan:
			klog.Infof("read stop at %v", time.Now())
			return
		}
	}
}
