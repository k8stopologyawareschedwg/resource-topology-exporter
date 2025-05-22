package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/config"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/k8shelpers"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/metrics"
	metricssrv "github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/metrics/server"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres/middleware/podexclude"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres/middleware/sharedcpuspool"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres/middleware/terminalpods"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcemonitor"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcetopologyexporter"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/version"
)

func main() {
	klog.Infof("starting %s version %s", version.ProgramName, version.Get())
	defer klog.Infof("stopped %s version %s", version.ProgramName, version.Get())

	parsedArgs, err := config.LoadArgs(os.Args[1:]...)
	if err != nil {
		klog.Fatalf("failed to parse args: %v", err)
	}

	if parsedArgs.DumpConfig != "" {
		conf := parsedArgs.ToYAMLString()

		if parsedArgs.DumpConfig == "-" {
			fmt.Println(conf)
		} else if parsedArgs.DumpConfig == ".andexit" {
			fmt.Println(conf)
			os.Exit(0)
		} else if parsedArgs.DumpConfig == ".log" {
			klog.Infof("current configuration:\n%s", conf)
		} else {
			err = os.WriteFile(parsedArgs.DumpConfig, []byte(conf), 0644)
			if err != nil {
				klog.Fatalf("failed to write the config to %q: %v", parsedArgs.DumpConfig, err)
			}
		}
	}

	if parsedArgs.Version {
		fmt.Println(version.ProgramName, version.Get())
		os.Exit(0)
	}

	// do as early as possible to make sure to fix the logger
	err = metrics.SetupWithEnviron(metrics.Environ{
		Logger: textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(parsedArgs.Global.Verbose))),
	})
	if err != nil {
		klog.Fatalf("failed to setup metrics: %v", err)
	}

	k8scli, err := k8shelpers.GetK8sClient(parsedArgs.Global.KubeConfig)
	if err != nil {
		klog.Fatalf("failed to get a kubernetes core client: %v", err)
	}

	nrtcli, err := k8shelpers.GetTopologyClient(parsedArgs.Global.KubeConfig)
	if err != nil {
		klog.Fatalf("failed to get a noderesourcetopology client: %v", err)
	}

	cli, cleanup, err := podres.WaitForReady(podres.GetClient(parsedArgs.RTE.PodResourcesSocketPath))
	if err != nil {
		klog.Fatalf("failed to get podresources client: %v", err)
	}
	defer cleanup()

	cli = sharedcpuspool.NewFromLister(cli, parsedArgs.Global.Debug, parsedArgs.RTE.ReferenceContainer)

	if len(parsedArgs.Resourcemonitor.PodExclude) > 0 {
		cli = podexclude.NewFromLister(cli, parsedArgs.Global.Debug, parsedArgs.Resourcemonitor.PodExclude)
	}

	if parsedArgs.Resourcemonitor.ExcludeTerminalPods {
		klog.Infof("terminal pods are filtered from the PodResourcesLister client")
		cli, err = terminalpods.NewFromLister(context.TODO(), cli, k8scli, time.Minute, parsedArgs.Global.Debug)
		if err != nil {
			klog.Fatalf("failed to get PodResourceAPI client: %v", err)
		}
	}

	err = metricssrv.Setup(parsedArgs.RTE.MetricsMode, metricssrv.NewConfig(parsedArgs.RTE.MetricsAddress, parsedArgs.RTE.MetricsPort, parsedArgs.RTE.MetricsTLSCfg))
	if err != nil {
		klog.Fatalf("failed to setup metrics server: %v", err)
	}

	hnd := resourcetopologyexporter.Handle{
		ResMon: resourcemonitor.Handle{
			PodResCli: cli,
			K8SCli:    k8scli,
		},
		NRTCli: nrtcli,
	}
	err = resourcetopologyexporter.Execute(hnd, parsedArgs.NRTupdater, parsedArgs.Resourcemonitor, parsedArgs.RTE)
	if err != nil {
		klog.Fatalf("failed to execute: %v", err)
	}
}
