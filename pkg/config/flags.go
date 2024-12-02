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
	"flag"
	"fmt"

	"k8s.io/klog/v2"

	metricssrv "github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/metrics/server"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres/middleware/sharedcpuspool"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcemonitor"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/version"
)

func FromFlags(pArgs *ProgArgs, args ...string) (string, string, error) {
	var refCnt string
	var configPath string

	flags := flag.NewFlagSet(version.ProgramName, flag.ExitOnError)

	klog.InitFlags(flags)

	flags.StringVar(&configPath, "config", LegacyExtraConfigPath, "Configuration file path. Use this to set the exclude list.")

	flags.BoolVar(&pArgs.Global.Debug, "debug", pArgs.Global.Debug, " Enable debug output.")
	flags.StringVar(&pArgs.Global.KubeConfig, "kubeconfig", pArgs.Global.KubeConfig, "path to kubeconfig file.")

	flags.BoolVar(&pArgs.NRTupdater.NoPublish, "no-publish", pArgs.NRTupdater.NoPublish, "Do not publish discovered features to the cluster-local Kubernetes API server.")
	flags.BoolVar(&pArgs.NRTupdater.Oneshot, "oneshot", pArgs.NRTupdater.Oneshot, "Update once and exit.")
	flags.StringVar(&pArgs.NRTupdater.Hostname, "hostname", pArgs.NRTupdater.Hostname, "Override the node hostname.")

	flags.StringVar(&pArgs.Resourcemonitor.Namespace, "watch-namespace", pArgs.Resourcemonitor.Namespace, "Namespace to watch pods for. Use \"\" for all namespaces.")
	flags.StringVar(&pArgs.Resourcemonitor.SysfsRoot, "sysfs", pArgs.Resourcemonitor.SysfsRoot, "Top-level component path of sysfs.")
	flags.BoolVar(&pArgs.Resourcemonitor.PodSetFingerprint, "pods-fingerprint", pArgs.Resourcemonitor.PodSetFingerprint, "If enable, compute and report the pod set fingerprint.")
	flags.BoolVar(&pArgs.Resourcemonitor.ExposeTiming, "expose-timing", pArgs.Resourcemonitor.ExposeTiming, "If enable, expose expected and actual sleep interval as annotations.")
	flags.BoolVar(&pArgs.Resourcemonitor.RefreshNodeResources, "refresh-node-resources", pArgs.Resourcemonitor.RefreshNodeResources, "If enable, track changes in node's resources")
	flags.StringVar(&pArgs.Resourcemonitor.PodSetFingerprintStatusFile, "pods-fingerprint-status-file", pArgs.Resourcemonitor.PodSetFingerprintStatusFile, "File to dump the pods fingerprint status. Use empty string to disable.")
	flags.BoolVar(&pArgs.Resourcemonitor.ExcludeTerminalPods, "exclude-terminal-pods", pArgs.Resourcemonitor.ExcludeTerminalPods, "If enable, exclude terminal pods from podresource API List call")
	flags.StringVar(&pArgs.Resourcemonitor.PodSetFingerprintMethod, "pods-fingerprint-method", pArgs.Resourcemonitor.PodSetFingerprintMethod, fmt.Sprintf("Select the method to compute the pods fingerprint. Valid options: %s.", resourcemonitor.PFPMethodSupported()))

	flags.StringVar(&pArgs.RTE.TopologyManagerPolicy, "topology-manager-policy", pArgs.RTE.TopologyManagerPolicy, "Explicitly set the topology manager policy instead of reading from the kubelet.")
	flags.StringVar(&pArgs.RTE.TopologyManagerScope, "topology-manager-scope", pArgs.RTE.TopologyManagerScope, "Explicitly set the topology manager scope instead of reading from the kubelet.")
	flags.DurationVar(&pArgs.RTE.SleepInterval, "sleep-interval", pArgs.RTE.SleepInterval, "Time to sleep between podresources API polls. Set to zero to completely disable the polling.")
	flags.StringVar(&pArgs.RTE.KubeletConfigFile, "kubelet-config-file", pArgs.RTE.KubeletConfigFile, "Kubelet config file path.")
	flags.StringVar(&pArgs.RTE.PodResourcesSocketPath, "podresources-socket", pArgs.RTE.PodResourcesSocketPath, "Pod Resource Socket path to use.")
	flags.BoolVar(&pArgs.RTE.PodReadinessEnable, "podreadiness", pArgs.RTE.PodReadinessEnable, "Custom condition injection using Podreadiness.")
	flags.BoolVar(&pArgs.RTE.AddNRTOwnerEnable, "add-nrt-owner", pArgs.RTE.AddNRTOwnerEnable, "RTE will inject NRT's related node as OwnerReference to ensure cleanup if the node is deleted.")
	flags.StringVar(&pArgs.RTE.MetricsMode, "metrics-mode", pArgs.RTE.MetricsMode, fmt.Sprintf("Select the mode to expose metrics endpoint. Valid options: %s", metricssrv.ServingModeSupported()))
	flags.IntVar(&pArgs.RTE.MetricsPort, "metrics-port", pArgs.RTE.MetricsPort, "Select the port to listen for the metrics endpoint.")
	flags.StringVar(&pArgs.RTE.MetricsAddress, "metrics-ip", pArgs.RTE.MetricsAddress, "Select the IP to listen for the metrics endpoint.")
	flags.StringVar(&pArgs.RTE.MetricsTLSCfg.CertsDir, "metrics-certs-dir", pArgs.RTE.MetricsTLSCfg.CertsDir, "certificates directory for TLS metrics serving")
	flags.StringVar(&pArgs.RTE.MetricsTLSCfg.CertFile, "metrics-cert-file", pArgs.RTE.MetricsTLSCfg.CertFile, "certificate file name for TLS metrics serving")
	flags.StringVar(&pArgs.RTE.MetricsTLSCfg.KeyFile, "metrics-key-file", pArgs.RTE.MetricsTLSCfg.KeyFile, "key file name for TLS metrics serving")
	flags.BoolVar(&pArgs.RTE.MetricsTLSCfg.WantCliAuth, "metrics-want-cli-auth", pArgs.RTE.MetricsTLSCfg.WantCliAuth, "Toggle if client certificate and authentication is required")

	flags.StringVar(&refCnt, "reference-container", pArgs.RTE.ReferenceContainer.String(), "Reference container, used to learn about the shared cpu pool\n See: https://github.com/kubernetes/kubernetes/issues/102190\n format of spec is namespace/podname/containername.\n Alternatively, you can use the env vars REFERENCE_NAMESPACE, REFERENCE_POD_NAME, REFERENCE_CONTAINER_NAME.")

	flags.StringVar(&pArgs.RTE.NotifyFilePath, "notify-file", pArgs.RTE.NotifyFilePath, "Notification file path.")
	// Lets keep it simple by now and expose only "events-per-second"
	// but logic is prepared to be able to also define the time base
	// that is why TimeUnitToLimitEvents is hard initialized to Second
	flags.Int64Var(&pArgs.RTE.MaxEventsPerTimeUnit, "max-events-per-second", pArgs.RTE.MaxEventsPerTimeUnit, "Max times per second resources will be scanned and updated")

	flags.BoolVar(&pArgs.Version, "version", pArgs.Version, "Output version and exit")
	flags.StringVar(&pArgs.DumpConfig, "dump-config", pArgs.DumpConfig, `dump the current configuration to the given file path. Empty string (default) disable the dumping.
Special targets:
. "-" for stdout.
. ".andexit" stdout and exit right after.
. ".log" to dump in the log".`,
	)

	err := flags.Parse(args)
	if err != nil {
		return DefaultConfigRoot, LegacyExtraConfigPath, err
	}

	if pArgs.Version {
		return DefaultConfigRoot, LegacyExtraConfigPath, err
	}

	if pArgs.Global.Debug {
		klog.Infof("using reference container: %q", refCnt)
	}
	if refCnt != "" {
		pArgs.RTE.ReferenceContainer, err = sharedcpuspool.ContainerIdentFromString(refCnt)
		if err != nil {
			return DefaultConfigRoot, LegacyExtraConfigPath, err
		}
	}
	if pArgs.Global.Debug {
		klog.Infof("reference container: %q", pArgs.RTE.ReferenceContainer.String())
	}

	params := flags.Args()
	if len(params) > 1 {
		return DefaultConfigRoot, configPath, fmt.Errorf("too many config roots given (%d), currently supported up to 1", len(params))
	}
	if len(params) == 0 {
		return DefaultConfigRoot, configPath, nil
	}
	configRoot := params[0]
	return configRoot, FixExtraConfigPath(configRoot), nil
}
