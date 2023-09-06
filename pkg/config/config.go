/*
Copyright 2022 The Kubernetes Authors.

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

package config

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"

	"github.com/k8stopologyawareschedwg/podfingerprint"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/nrtupdater"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres/middleware/podexclude"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres/middleware/sharedcpuspool"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcemonitor"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcetopologyexporter"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/version"
)

type ProgArgs struct {
	NRTupdater      nrtupdater.Args
	Resourcemonitor resourcemonitor.Args
	RTE             resourcetopologyexporter.Args
	Version         bool
	DumpConfig      string
}

func (pa *ProgArgs) ToJson() ([]byte, error) {
	return json.Marshal(pa)
}

func (pa *ProgArgs) ToYaml() ([]byte, error) {
	return yaml.Marshal(pa)
}

type kubeletParams struct {
	TopologyManagerPolicy string `json:"topologyManagerPolicy,omitempty"`
	TopologyManagerScope  string `json:"topologyManagerScope,omitempty"`
}

type config struct {
	Kubelet         kubeletParams                   `json:"kubelet,omitempty"`
	ResourceExclude resourcemonitor.ResourceExclude `json:"resourceExclude,omitempty"`
	PodExclude      podexclude.List                 `json:"podExclude,omitempty"`
}

func readConfig(configPath string) (config, error) {
	conf := config{}
	data, err := os.ReadFile(configPath)
	if err != nil {
		// config is optional
		if errors.Is(err, os.ErrNotExist) {
			klog.Infof("couldn't find configuration in %q", configPath)
			return conf, nil
		}
		return conf, err
	}
	err = yaml.Unmarshal(data, &conf)
	return conf, err
}

// The args is passed only for testing purposes.
func LoadArgs(args ...string) (ProgArgs, error) {
	var pArgs ProgArgs

	var configPath string
	var pfpMethod string
	flags := flag.NewFlagSet(version.ProgramName, flag.ExitOnError)

	klog.InitFlags(flags)

	flags.BoolVar(&pArgs.NRTupdater.NoPublish, "no-publish", false, "Do not publish discovered features to the cluster-local Kubernetes API server.")
	flags.BoolVar(&pArgs.NRTupdater.Oneshot, "oneshot", false, "Update once and exit.")
	flags.StringVar(&pArgs.NRTupdater.Hostname, "hostname", DefaultHostName(), "Override the node hostname.")

	flags.StringVar(&pArgs.Resourcemonitor.Namespace, "watch-namespace", "", "Namespace to watch pods for. Use \"\" for all namespaces.")
	flags.StringVar(&pArgs.Resourcemonitor.SysfsRoot, "sysfs", "/sys", "Top-level component path of sysfs.")
	flags.BoolVar(&pArgs.Resourcemonitor.PodSetFingerprint, "pods-fingerprint", true, "If enable, compute and report the pod set fingerprint.")
	flags.BoolVar(&pArgs.Resourcemonitor.ExposeTiming, "expose-timing", false, "If enable, expose expected and actual sleep interval as annotations.")
	flags.BoolVar(&pArgs.Resourcemonitor.RefreshNodeResources, "refresh-node-resources", false, "If enable, track changes in node's resources")
	flags.StringVar(&pArgs.Resourcemonitor.PodSetFingerprintStatusFile, "pods-fingerprint-status-file", "", "File to dump the pods fingerprint status. Use empty string to disable.")
	flags.BoolVar(&pArgs.Resourcemonitor.ExcludeTerminalPods, "exclude-terminal-pods", false, "If enable, exclude terminal pods from podresource API List call")
	flags.StringVar(&pfpMethod, "pods-fingerprint-method", podfingerprint.MethodWithExclusiveResources, fmt.Sprintf("Select the method to compute the pods fingerprint. Valid options: %s.", resourcemonitor.PFPMethodSupported()))

	flags.StringVar(&configPath, "config", "/etc/resource-topology-exporter/config.yaml", "Configuration file path. Use this to set the exclude list.")

	flags.BoolVar(&pArgs.RTE.Debug, "debug", false, " Enable debug output.")
	flags.StringVar(&pArgs.RTE.TopologyManagerPolicy, "topology-manager-policy", DefaultTopologyManagerPolicy(), "Explicitly set the topology manager policy instead of reading from the kubelet.")
	flags.StringVar(&pArgs.RTE.TopologyManagerScope, "topology-manager-scope", DefaultTopologyManagerScope(), "Explicitly set the topology manager scope instead of reading from the kubelet.")
	flags.DurationVar(&pArgs.RTE.SleepInterval, "sleep-interval", 60*time.Second, "Time to sleep between podresources API polls. Set to zero to completely disable the polling.")
	flags.StringVar(&pArgs.RTE.KubeletConfigFile, "kubelet-config-file", "/podresources/config.yaml", "Kubelet config file path.")
	flags.StringVar(&pArgs.RTE.PodResourcesSocketPath, "podresources-socket", "unix:///podresources/kubelet.sock", "Pod Resource Socket path to use.")
	flags.BoolVar(&pArgs.RTE.PodReadinessEnable, "podreadiness", true, "Custom condition injection using Podreadiness.")
	flags.BoolVar(&pArgs.RTE.AddNRTOwnerEnable, "add-nrt-owner", true, "RTE will inject NRT's related node as OwnerReference to ensure cleanup if the node is deleted.")

	refCnt := flags.String("reference-container", "", "Reference container, used to learn about the shared cpu pool\n See: https://github.com/kubernetes/kubernetes/issues/102190\n format of spec is namespace/podname/containername.\n Alternatively, you can use the env vars REFERENCE_NAMESPACE, REFERENCE_POD_NAME, REFERENCE_CONTAINER_NAME.")

	flags.StringVar(&pArgs.RTE.NotifyFilePath, "notify-file", "", "Notification file path.")
	// Lets keep it simple by now and expose only "events-per-second"
	// but logic is prepared to be able to also define the time base
	// that is why TimeUnitToLimitEvents is hard initialized to Second
	flags.Int64Var(&pArgs.RTE.MaxEventsPerTimeUnit, "max-events-per-second", 1, "Max times per second resources will be scanned and updated")
	pArgs.RTE.TimeUnitToLimitEvents = time.Second

	flags.BoolVar(&pArgs.Version, "version", false, "Output version and exit")
	flags.StringVar(&pArgs.DumpConfig, "dump-config", "",
		`dump the current configuration to the given file path. Empty string (default) disable the dumping.
Special targets:
. "-" for stdout.
. ".andexit" stdout and exit right after.
. ".log" to dump in the log".`,
	)

	err := flags.Parse(args)
	if err != nil {
		return pArgs, err
	}

	if pArgs.Version {
		return pArgs, err
	}

	pArgs.RTE.ReferenceContainer, err = setContainerIdent(*refCnt)
	if err != nil {
		return pArgs, err
	}
	if pArgs.RTE.ReferenceContainer.IsEmpty() {
		pArgs.RTE.ReferenceContainer = sharedcpuspool.ContainerIdentFromEnv()
	}

	pArgs.Resourcemonitor.PodSetFingerprintMethod, err = resourcemonitor.PFPMethodIsSupported(pfpMethod)
	if err != nil {
		return pArgs, err
	}

	conf, err := readConfig(configPath)
	if err != nil {
		return pArgs, fmt.Errorf("error getting exclude list from the configuration: %w", err)
	}

	err = setupArgsFromConfig(&pArgs, conf)
	return pArgs, err
}

func setupArgsFromConfig(pArgs *ProgArgs, conf config) error {
	if len(conf.ResourceExclude) > 0 {
		pArgs.Resourcemonitor.ResourceExclude = conf.ResourceExclude
		klog.V(2).Infof("using resources exclude:\n%s", pArgs.Resourcemonitor.ResourceExclude.String())
	}

	if len(conf.PodExclude) > 0 {
		pArgs.Resourcemonitor.PodExclude = conf.PodExclude
		klog.V(2).Infof("using pod excludes:\n%s", pArgs.Resourcemonitor.PodExclude.String())
	}

	if pArgs.RTE.TopologyManagerPolicy == "" {
		pArgs.RTE.TopologyManagerPolicy = conf.Kubelet.TopologyManagerPolicy
		klog.V(2).Infof("using kubelet topology manager policy: %q", pArgs.RTE.TopologyManagerPolicy)
	}
	if pArgs.RTE.TopologyManagerScope == "" {
		pArgs.RTE.TopologyManagerScope = conf.Kubelet.TopologyManagerScope
		klog.V(2).Infof("using kubelet topology manager scope: %q", pArgs.RTE.TopologyManagerScope)
	}

	return nil
}

func setContainerIdent(value string) (*sharedcpuspool.ContainerIdent, error) {
	ci, err := sharedcpuspool.ContainerIdentFromString(value)
	if err != nil {
		return nil, err
	}

	if ci == nil {
		return &sharedcpuspool.ContainerIdent{}, nil
	}

	return ci, nil
}
