/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package resourcetopologyexporter

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"

	"k8s.io/klog/v2"
	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/kubeletconfig"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/nrtupdater"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podrescli"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/prometheus"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcemonitor"
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

	upd, err := nrtupdater.NewNRTUpdater(nrtupdaterArgs, tmPolicy)
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

func getTopologyManagerPolicy(resourcemonitorArgs resourcemonitor.Args, rteArgs Args) (string, error) {
	if rteArgs.TopologyManagerPolicy != "" {
		klog.Infof("using given Topology Manager policy %q", rteArgs.TopologyManagerPolicy)
		return rteArgs.TopologyManagerPolicy, nil
	}
	if rteArgs.KubeletConfigFile != "" {
		klConfig, err := kubeletconfig.GetKubeletConfigFromLocalFile(rteArgs.KubeletConfigFile)
		if err != nil {
			return "", fmt.Errorf("error getting topology Manager Policy: %w", err)
		}
		klog.Infof("detected kubelet Topology Manager policy %q", klConfig.TopologyManagerPolicy)
		return klConfig.TopologyManagerPolicy, nil
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
