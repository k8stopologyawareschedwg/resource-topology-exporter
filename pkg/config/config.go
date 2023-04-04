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
	"strings"
	"time"

	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/nrtupdater"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podrescli"
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

type config struct {
	ExcludeList resourcemonitor.ResourceExcludeList
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
	flags := flag.NewFlagSet(version.ProgramName, flag.ExitOnError)

	klog.InitFlags(flags)

	flags.BoolVar(&pArgs.NRTupdater.NoPublish, "no-publish", false, "Do not publish discovered features to the cluster-local Kubernetes API server.")
	flags.BoolVar(&pArgs.NRTupdater.Oneshot, "oneshot", false, "Update once and exit.")
	flags.StringVar(&pArgs.NRTupdater.Hostname, "hostname", DefaultHostName(), "Override the node hostname.")

	flags.StringVar(&pArgs.Resourcemonitor.Namespace, "watch-namespace", "", "Namespace to watch pods for. Use \"\" for all namespaces.")
	flags.StringVar(&pArgs.Resourcemonitor.SysfsRoot, "sysfs", "/sys", "Top-level component path of sysfs.")
	flags.BoolVar(&pArgs.Resourcemonitor.PodSetFingerprint, "pods-fingerprint", false, "If enable, compute and report the pod set fingerprint.")
	flags.BoolVar(&pArgs.Resourcemonitor.ExposeTiming, "expose-timing", false, "If enable, expose expected and actual sleep interval as annotations.")
	flags.BoolVar(&pArgs.Resourcemonitor.RefreshNodeResources, "refresh-node-resources", false, "If enable, track changes in node's resources")
	flags.StringVar(&pArgs.Resourcemonitor.PodSetFingerprintStatusFile, "pods-fingerprint-status-file", "", "File to dump the pods fingerprint status. Use empty string to disable.")

	flags.StringVar(&configPath, "config", "/etc/resource-topology-exporter/config.yaml", "Configuration file path. Use this to set the exclude list.")

	flags.BoolVar(&pArgs.RTE.Debug, "debug", false, " Enable debug output.")
	flags.StringVar(&pArgs.RTE.TopologyManagerPolicy, "topology-manager-policy", DefaultTopologyManagerPolicy(), "Explicitly set the topology manager policy instead of reading from the kubelet.")
	flags.StringVar(&pArgs.RTE.TopologyManagerScope, "topology-manager-scope", DefaultTopologyManagerScope(), "Explicitly set the topology manager scope instead of reading from the kubelet.")
	flags.DurationVar(&pArgs.RTE.SleepInterval, "sleep-interval", 60*time.Second, "Time to sleep between podresources API polls. Set to zero to completely disable the polling.")
	flags.StringVar(&pArgs.RTE.KubeletConfigFile, "kubelet-config-file", "/podresources/config.yaml", "Kubelet config file path.")
	flags.StringVar(&pArgs.RTE.PodResourcesSocketPath, "podresources-socket", "unix:///podresources/kubelet.sock", "Pod Resource Socket path to use.")
	flags.BoolVar(&pArgs.RTE.PodReadinessEnable, "podreadiness", true, "Custom condition injection using Podreadiness.")

	kubeletStateDirs := flags.String("kubelet-state-dir", "", "Kubelet state directory (RO access needed), for smart polling. **DEPRECATED** please use notify-file")
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

	conf, err := readConfig(configPath)
	if err != nil {
		return pArgs, fmt.Errorf("error getting exclude list from the configuration: %w", err)
	}

	if len(conf.ExcludeList) != 0 {
		pArgs.Resourcemonitor.ExcludeList = conf.ExcludeList
		klog.V(2).Infof("using exclude list:\n%s", pArgs.Resourcemonitor.ExcludeList.String())
	}

	pArgs.RTE.KubeletStateDirs, err = setKubeletStateDirs(*kubeletStateDirs)
	if err != nil {
		return pArgs, err
	}

	pArgs.RTE.ReferenceContainer, err = setContainerIdent(*refCnt)
	if err != nil {
		return pArgs, err
	}
	if pArgs.RTE.ReferenceContainer.IsEmpty() {
		pArgs.RTE.ReferenceContainer = podrescli.ContainerIdentFromEnv()
	}

	return pArgs, nil
}

func setKubeletStateDirs(value string) ([]string, error) {
	ksd := make([]string, 0)
	for _, s := range strings.Split(value, " ") {
		ksd = append(ksd, s)
	}
	return ksd, nil
}

func setContainerIdent(value string) (*podrescli.ContainerIdent, error) {
	ci, err := podrescli.ContainerIdentFromString(value)
	if err != nil {
		return nil, err
	}

	if ci == nil {
		return &podrescli.ContainerIdent{}, nil
	}

	return ci, nil
}
