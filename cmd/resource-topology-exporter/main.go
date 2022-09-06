package main

import (
	"fmt"
	"os"

	"k8s.io/klog/v2"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/config"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podrescli"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/prometheus"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcetopologyexporter"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/version"
)

func main() {
	cfg, err := config.LoadArgs(os.Args[1:]...)
	if err != nil {
		klog.Fatalf("failed to parse args: %v", err)
	}

	if cfg.Core.DumpConfig != "" {
		data, err := cfg.ToYaml()
		if err != nil {
			klog.Fatalf("failed to marshal the config: %v", err)
		}

		if cfg.Core.DumpConfig == "-" {
			fmt.Println(string(data))
		} else if cfg.Core.DumpConfig == ".andexit" {
			fmt.Println(string(data))
			os.Exit(0)
		} else if cfg.Core.DumpConfig == ".log" {
			klog.Infof("current configuration:\n%s", string(data))
		} else {
			err = os.WriteFile(cfg.Core.DumpConfig, data, 0644)
			if err != nil {
				klog.Fatalf("failed to write the config to %q: %v", cfg.Core.DumpConfig, err)
			}
		}
	}

	if cfg.Core.Version {
		fmt.Println(version.ProgramName, version.Get())
		os.Exit(0)
	}

	cli, err := podrescli.NewFilteringClient(cfg.RTE.PodResourcesSocketPath, cfg.Core.Debug, cfg.RTE.ReferenceContainer)
	if err != nil {
		klog.Fatalf("failed to get podresources client: %v", err)
	}

	err = prometheus.InitPrometheus()
	if err != nil {
		klog.Fatalf("failed to start prometheus server: %v", err)
	}

	err = resourcetopologyexporter.Execute(cli, cfg.NRTupdater, cfg.Resourcemonitor, cfg.RTE)
	if err != nil {
		klog.Fatalf("failed to execute: %v", err)
	}
}
