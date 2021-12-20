package main

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
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/prometheus"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcemonitor"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcetopologyexporter"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/version"
)

type ProgArgs struct {
	NRTupdater      nrtupdater.Args
	Resourcemonitor resourcemonitor.Args
	RTE             resourcetopologyexporter.Args
	Version         bool
}

func (pa *ProgArgs) ToJson() ([]byte, error) {
	return json.Marshal(pa)
}

func main() {
	parsedArgs, err := parseArgs(os.Args[1:]...)
	if err != nil {
		klog.Fatalf("failed to parse args: %v", err)
	}

	if parsedArgs.Version {
		fmt.Println(version.ProgramName, version.Get())
		os.Exit(0)
	}

	cli, err := podrescli.NewFilteringClient(parsedArgs.RTE.PodResourcesSocketPath, parsedArgs.RTE.Debug, parsedArgs.RTE.ReferenceContainer)
	if err != nil {
		klog.Fatalf("failed to get podresources client: %v", err)
	}

	err = prometheus.InitPrometheus()
	if err != nil {
		klog.Fatalf("failed to start prometheus server: %v", err)
	}

	err = resourcetopologyexporter.Execute(cli, parsedArgs.NRTupdater, parsedArgs.Resourcemonitor, parsedArgs.RTE)
	if err != nil {
		klog.Fatalf("failed to execute: %v", err)
	}
}

type config struct {
	ExcludeList map[string][]string
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
func parseArgs(args ...string) (ProgArgs, error) {
	pArgs := ProgArgs{
		nrtupdater.Args{},
		resourcemonitor.Args{},
		resourcetopologyexporter.Args{},
		false,
	}

	var configPath string
	flags := flag.NewFlagSet(version.ProgramName, flag.ExitOnError)

	klog.InitFlags(flags)

	flags.BoolVar(&pArgs.NRTupdater.NoPublish, "no-publish", false, "Do not publish discovered features to the cluster-local Kubernetes API server.")
	flags.BoolVar(&pArgs.NRTupdater.Oneshot, "oneshot", false, "Update once and exit.")
	flags.StringVar(&pArgs.NRTupdater.Hostname, "hostname", defaultHostName(), "Override the node hostname.")

	flags.StringVar(&pArgs.Resourcemonitor.Namespace, "watch-namespace", "", "Namespace to watch pods for. Use \"\" for all namespaces.")
	flags.StringVar(&pArgs.Resourcemonitor.SysfsRoot, "sysfs", "/sys", "Top-level component path of sysfs.")

	flags.StringVar(&configPath, "config", "/etc/resource-topology-exporter/config.yaml", "Configuration file path. Use this to set the exclude list.")

	flags.BoolVar(&pArgs.RTE.Debug, "debug", false, " Enable debug output.")
	flags.StringVar(&pArgs.RTE.TopologyManagerPolicy, "topology-manager-policy", defaultTopologyManagerPolicy(), "Explicitly set the topology manager policy instead of reading from the kubelet.")
	flags.StringVar(&pArgs.RTE.TopologyManagerScope, "topology-manager-scope", defaultTopologyManagerScope(), "Explicitly set the topology manager scope instead of reading from the kubelet.")
	flags.DurationVar(&pArgs.RTE.SleepInterval, "sleep-interval", 60*time.Second, "Time to sleep between podresources API polls.")
	flags.StringVar(&pArgs.RTE.KubeletConfigFile, "kubelet-config-file", "/podresources/config.yaml", "Kubelet config file path.")
	flags.StringVar(&pArgs.RTE.PodResourcesSocketPath, "podresources-socket", "unix:///podresources/kubelet.sock", "Pod Resource Socket path to use.")
	flags.BoolVar(&pArgs.RTE.PodReadinessEnable, "podreadiness", true, "Custom condition injection using Podreadiness.")

	kubeletStateDirs := flags.String("kubelet-state-dir", "", "Kubelet state directory (RO access needed), for smart polling.")
	refCnt := flags.String("reference-container", "", "Reference container, used to learn about the shared cpu pool\n See: https://github.com/kubernetes/kubernetes/issues/102190\n format of spec is namespace/podname/containername.\n Alternatively, you can use the env vars REFERENCE_NAMESPACE, REFERENCE_POD_NAME, REFERENCE_CONTAINER_NAME.")

	flags.StringVar(&pArgs.RTE.NotifyFilePath, "notify-file", "", "Notification file path.")
	// Lets keep it simple by now and expose only "events-per-second"
	// but logic is prepared to be able to also define the time base
	// that is why TimeUnitToLimitEvents is hard initialized to Second
	flags.Int64Var(&pArgs.RTE.MaxEventsPerTimeUnit, "max-events-per-second", 1, "Max times per second resources will be scanned and updated")
	pArgs.RTE.TimeUnitToLimitEvents = time.Second

	flags.BoolVar(&pArgs.Version, "version", false, "Output version and exit")

	err := flags.Parse(args)
	if err != nil {
		return pArgs, err
	}

	if pArgs.Version {
		return pArgs, err
	}

	conf, err := readConfig(configPath)
	if err != nil {
		return pArgs, fmt.Errorf("error getting exclude list from the configuration: %v", err)
	}

	if len(conf.ExcludeList) != 0 {
		pArgs.Resourcemonitor.ExcludeList.ExcludeList = conf.ExcludeList
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

func defaultHostName() string {
	var err error

	val, ok := os.LookupEnv("NODE_NAME")
	if !ok || val == "" {
		val, err = os.Hostname()
		if err != nil {
			klog.Fatalf("error getting the host name: %v", err)
		}
	}
	return val
}

func defaultTopologyManagerPolicy() string {
	if val, ok := os.LookupEnv("TOPOLOGY_MANAGER_POLICY"); ok {
		return val
	}
	// empty string is a valid value here, so just keep going
	return ""
}

func defaultTopologyManagerScope() string {
	if val, ok := os.LookupEnv("TOPOLOGY_MANAGER_SCOPE"); ok {
		return val
	}
	// empty string is a valid value here, so just keep going
	return ""
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
