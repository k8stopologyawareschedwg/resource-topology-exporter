package resourcetopologyexporter

import (
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"

	v1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/kubeconf"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/notification"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/nrtupdater"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podreadiness"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podrescli"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/ratelimiter"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcemonitor"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/topologypolicy"
)

type Args struct {
	KubeletConfigFile      string                    `koanf:"kubelet-config-file" json:"kubelet-config-file"`
	KubeletStateDirs       []string                  `koanf:"kubelet-state-dir" json:"kubelet-state-dir,omitempty"`
	MaxEventsPerTimeUnit   int64                     `koanf:"max-events-per-second" json:"max-events-per-second"`
	NotifyFilePath         string                    `koanf:"notify-file" json:"notify-file,omitempty"`
	PodReadinessEnable     bool                      `koanf:"podreadiness" json:"podreadiness"`
	PodResourcesSocketPath string                    `koanf:"podresources-socket" json:"podresources-socket"`
	ReferenceContainer     *podrescli.ContainerIdent `koanf:"reference-container" json:"reference-container,omitempty"`
	SleepInterval          time.Duration             `koanf:"sleep-interval" json:"sleep-interval"`
	TopologyManagerPolicy  string                    `koanf:"topology-manager-policy" json:"topology-manager-policy,omitempty"`
	TopologyManagerScope   string                    `koanf:"topology-manager-scope" json:"topology-manager-scope,omitempty"`
	TimeUnitToLimitEvents  time.Duration             `koanf:"time-unit-to-limit-events" json:"-"`
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

	eventSource, err := createEventSource(&rteArgs)
	if err != nil {
		return err
	}

	resObs, err := NewResourceObserver(cli, resourcemonitorArgs)
	if err != nil {
		return err
	}
	go resObs.Run(eventSource.Events(), condChan)

	upd := nrtupdater.NewNRTUpdater(nrtupdaterArgs, string(tmPolicy))
	go upd.Run(resObs.Infos, condChan)

	go eventSource.Run()

	eventSource.Wait()  // will never return
	eventSource.Close() // still we try to clean after ourselves :)
	return nil          // unreachable
}

func createEventSource(rteArgs *Args) (notification.EventSource, error) {
	var es notification.EventSource

	eventSource, err := notification.NewUnlimitedEventSource(rteArgs.SleepInterval)
	if err != nil {
		return nil, err
	}

	err = eventSource.AddFile(rteArgs.NotifyFilePath)
	if err != nil {
		return nil, err
	}

	err = eventSource.AddDirs(rteArgs.KubeletStateDirs)
	if err != nil {
		return nil, err
	}

	es = eventSource

	// If rate limit parameters are configured set it up
	if rteArgs.MaxEventsPerTimeUnit > 0 && rteArgs.TimeUnitToLimitEvents > 0 {
		es, err = ratelimiter.NewRateLimitedEventSource(eventSource, uint64(rteArgs.MaxEventsPerTimeUnit), rteArgs.TimeUnitToLimitEvents)
		if err != nil {
			return nil, err
		}
	}

	return es, nil
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
