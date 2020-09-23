package main

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/docopt/docopt-go"
	"log"
	"time"

	"github.com/swatisehgal/resource-topology-exporter/pkg/exporter"
	"github.com/swatisehgal/resource-topology-exporter/pkg/finder"
	"github.com/swatisehgal/resource-topology-exporter/pkg/kubeconf"
	"github.com/swatisehgal/resource-topology-exporter/pkg/podres"
)

const (
	// ProgramName is the canonical name of this program
	ProgramName = "resource-topology-exporter"
)

func main() {
	// Parse command-line arguments.
	args, err := argsParse(nil)
	if err != nil {
		log.Fatalf("failed to parse command line: %v", err)
	}

	klConfig, err := kubeconf.GetKubeletConfigFromLocalFile(args.KubeletConfigFile)
	if err != nil {
		log.Fatalf("error getting topology Manager Policy: %v", err)
	}
	tmPolicy := klConfig.TopologyManagerPolicy
	log.Printf("Detected kubelet Topology Manager policy %q", tmPolicy)

	podResClient, err := podres.GetPodResClient(args.PodResourceSocketPath)
	if err != nil {
		log.Fatalf("Failed to get PodResource Client: %v", err)
	}
	var finderInstance finder.Finder

	finderInstance, err = finder.NewPodResourceFinder(args, podResClient)
	if err != nil {
		log.Fatalf("Failed to initialize Finder instance: %v", err)
	}
	crdExporter, err := exporter.NewExporter(tmPolicy)
	if err != nil {
		log.Fatalf("Failed to initialize crdExporter instance: %v", err)
	}

	// CAUTION: these resources are expected to change rarely - if ever.
	//So we are intentionally do this once during the process lifecycle.
	//TODO: Obtain node resources dynamically from the podresource API

	nodeResourceData, err := finder.NewNodeResources(args.SysfsRoot, podResClient)
	if err != nil {
		log.Fatalf("Failed to obtain node resource information: %v", err)
	}
	log.Printf("nodeResourceData is: %v\n", nodeResourceData)
	for {
		podResources, err := finderInstance.Scan(nodeResourceData.GetDeviceResourceMap())
		log.Printf("podResources is: %v\n", podResources)
		if err != nil {
			log.Printf("Scan failed: %v\n", err)
			continue
		}

		zones := finder.Aggregate(podResources, nodeResourceData)
		log.Printf("zones:%v", spew.Sdump(zones))

		if err = crdExporter.CreateOrUpdate("default", zones); err != nil {
			log.Fatalf("ERROR: %v", err)
		}

		time.Sleep(args.SleepInterval)
	}
}

// argsParse parses the command line arguments passed to the program.
// The argument argv is passed only for testing purposes.
func argsParse(argv []string) (finder.Args, error) {
	args := finder.Args{
		SleepInterval:     time.Duration(3 * time.Second),
		SysfsRoot:         "/host-sys",
		KubeletConfigFile: "/host-etc/kubernetes/kubelet.conf",
	}
	usage := fmt.Sprintf(`Usage:
  %s [--sleep-interval=<seconds>] [--source=<path>] [--container-runtime=<runtime>] [--cri-socket=<path>] [--podresources-socket=<path>] [--watch-namespace=<namespace>] [--sysfs=<mountpoint>] [--sriov-config-file=<path>] [--kubelet-config-file=<path>]
  %s -h | --help
  Options:
  -h --help                       Show this screen.
	--podresources-socket=<path>    Pod Resource Socket path to use.
  --sleep-interval=<seconds>      Time to sleep between updates. [Default: %v]
  --watch-namespace=<namespace>   Namespace to watch pods for. Use "" for all namespaces.
  --sysfs=<mountpoint>            Mount point of the sysfs. [Default: %v]
  --kubelet-config-file=<path>    Kubelet config file path. [Default: %v]`,
		ProgramName,
		ProgramName,
		args.SleepInterval,
		args.SysfsRoot,
		args.KubeletConfigFile,
	)

	arguments, _ := docopt.ParseArgs(usage, argv, ProgramName)
	var err error
	// Parse argument values as usable types.
	if ns, ok := arguments["--watch-namespace"].(string); ok {
		args.Namespace = ns
	}
	if kubeletConfigPath, ok := arguments["--kubelet-config-file"].(string); ok {
		args.KubeletConfigFile = kubeletConfigPath
	}
	args.SysfsRoot = arguments["--sysfs"].(string)
	if path, ok := arguments["--podresources-socket"].(string); ok {
		args.PodResourceSocketPath = path
	}
	args.SleepInterval, err = time.ParseDuration(arguments["--sleep-interval"].(string))
	if err != nil {
		return args, fmt.Errorf("invalid --sleep-interval specified: %s", err.Error())
	}
	return args, nil
}
