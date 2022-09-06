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
	parsedArgs, err := config.LoadArgs(os.Args[1:]...)
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
