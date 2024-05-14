package main

import (
	"flag"
	"time"

	"k8s.io/klog/v2"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/k8shelpers"
	nrtres "github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcegenerator/nrt"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourceupdater"
)

func main() {
	var hostname string
	var tmPolicy string
	var tmScope string
	var interval time.Duration
	var seed int
	var dryRun bool
	flag.StringVar(&hostname, "hosthame", "fake-host-0", "fake host name (not validated)")
	flag.StringVar(&tmPolicy, "tm-policy", "single-numa-node", "topology manager policy (not validated)")
	flag.StringVar(&tmScope, "tm-scope", "pod", "topology manager scope (not validated)")
	flag.DurationVar(&interval, "interval", 10*time.Second, "periodic update interval")
	flag.IntVar(&seed, "random-seed", 0, "random seed (use time)")
	flag.BoolVar(&dryRun, "dry-run", false, "don't send data")

	klog.InitFlags(nil)
	flag.Parse()

	resourceupdaterArgs := resourceupdater.Args{
		Hostname:  hostname,
		NoPublish: dryRun,
	}

	var randSeed int64 = time.Now().UnixNano()
	if seed != 0 {
		randSeed = int64(seed)
	}

	klog.Infof("generating fake periodic updates every %v with random seed %v", interval, randSeed)

	gen := nrtres.NewGenerator(interval, randSeed)
	go gen.Run()

	klog.Infof("using NRT Updater args: %+#v", resourceupdaterArgs)

	tmConf := resourceupdater.TMConfig{
		Policy: tmPolicy,
		Scope:  tmScope,
	}

	nrtcli, err := k8shelpers.GetTopologyClient("")
	if err != nil {
		klog.Fatalf("failed to get a noderesourcetopology client: %v", err)
	}

	nodeGetter := &resourceupdater.DisabledNodeGetter{}
	upd, err := resourceupdater.NewNRTUpdater(nodeGetter, nrtcli, resourceupdaterArgs, tmConf)
	if err != nil {
		klog.Fatalf("failed to create a noderesourcetopology updater: %v", err)
	}
	upd.Run(gen.Infos, nil)
}
