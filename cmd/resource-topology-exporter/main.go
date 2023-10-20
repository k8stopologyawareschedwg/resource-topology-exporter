package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"k8s.io/klog/v2"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/config"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/k8shelpers"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres/middleware/podexclude"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres/middleware/sharedcpuspool"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres/middleware/terminalpods"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/prometheus"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcetopologyexporter"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/version"
)

func main() {
	parsedArgs, err := config.LoadArgs(os.Args[1:]...)
	if err != nil {
		klog.Fatalf("failed to parse args: %v", err)
	}

	if parsedArgs.DumpConfig != "" {
		data, err := parsedArgs.ToYaml()
		if err != nil {
			klog.Fatalf("failed to marshal the config: %v", err)
		}

		if parsedArgs.DumpConfig == "-" {
			fmt.Println(string(data))
		} else if parsedArgs.DumpConfig == ".andexit" {
			fmt.Println(string(data))
			os.Exit(0)
		} else if parsedArgs.DumpConfig == ".log" {
			klog.Infof("current configuration:\n%s", string(data))
		} else {
			err = os.WriteFile(parsedArgs.DumpConfig, data, 0644)
			if err != nil {
				klog.Fatalf("failed to write the config to %q: %v", parsedArgs.DumpConfig, err)
			}
		}
	}

	if parsedArgs.Version {
		fmt.Println(version.ProgramName, version.Get())
		os.Exit(0)
	}

	cli, err := podres.GetClient(parsedArgs.RTE.PodResourcesSocketPath)
	if err != nil {
		klog.Fatalf("failed to get podresources client: %v", err)
	}

	cli = sharedcpuspool.NewFromLister(cli, parsedArgs.RTE.Debug, parsedArgs.RTE.ReferenceContainer)

	if len(parsedArgs.Resourcemonitor.PodExclude) > 0 {
		cli = podexclude.NewFromLister(cli, parsedArgs.RTE.Debug, parsedArgs.Resourcemonitor.PodExclude)
	}

	if parsedArgs.Resourcemonitor.ExcludeTerminalPods {
		klog.Infof("terminal pods are filtered from the PodResourcesLister client")
		k8scli, err := k8shelpers.GetK8sClient("")
		if err != nil {
			klog.Fatalf("failed to get k8s client: %w", err)
		}
		cli, err = terminalpods.NewFromLister(context.TODO(), cli, k8scli, time.Minute, parsedArgs.RTE.Debug)
		if err != nil {
			klog.Fatalf("failed to get PodResourceAPI client: %w", err)
		}
	}

	err = prometheus.InitPrometheus(parsedArgs.RTE.PrometheusMode)
	if err != nil {
		klog.Fatalf("failed to start prometheus server: %v", err)
	}

	err = resourcetopologyexporter.Execute(cli, parsedArgs.NRTupdater, parsedArgs.Resourcemonitor, parsedArgs.RTE)
	if err != nil {
		klog.Fatalf("failed to execute: %v", err)
	}
}
