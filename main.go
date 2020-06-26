package main

import (
	"fmt"
	"log"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/docopt/docopt-go"

	"github.com/swatisehgal/resource-topology-exporter/pkg/exporter"
	"github.com/swatisehgal/resource-topology-exporter/pkg/finder"
	"github.com/swatisehgal/resource-topology-exporter/pkg/pciconf"
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
	if args.SRIOVConfigFile == "" {
		log.Fatalf("missing SRIOV device plugin configuration file path")
	}

	var pci2ResMap pciconf.PCIResourceMap
	log.Printf("getting SRIOV configuration from file: %s", args.SRIOVConfigFile)
	pci2ResMap, err = pciconf.GetFromSRIOVConfigFile(args.SysfsRoot, args.SRIOVConfigFile)
	if err != nil {
		log.Fatalf("failed to read the PCI -> Resource mapping: %v", err)
	}

	// Get new finder instance
	instance, err := finder.NewFinder(args, pci2ResMap)
	if err != nil {
		log.Fatalf("Failed to initialize NfdWorker instance: %v", err)
	}

	crdExporter, err := exporter.NewExporter()
	if err != nil {
		log.Fatalf("Failed to initialize crdExporter instance: %v", err)
	}

	for {
		podResources, err := instance.Scan()
		if err != nil {
			log.Printf("CRI scan failed: %v\n", err)
			continue
		}

		perNumaResources := instance.Aggregate(podResources)
		log.Printf("allocatedResourcesNumaInfo:%v", spew.Sdump(perNumaResources))

		if err = crdExporter.CreateOrUpdate("default", perNumaResources); err != nil {
			log.Fatalf("ERROR: %v", err)
		}

		time.Sleep(args.SleepInterval)
	}
}

// argsParse parses the command line arguments passed to the program.
// The argument argv is passed only for testing purposes.
func argsParse(argv []string) (finder.Args, error) {
	args := finder.Args{
		CRIEndpointPath: "/host-run/containerd/containerd.sock",
		SleepInterval:   time.Duration(3 * time.Second),
		SysfsRoot:       "/host-sys",
	}
	usage := fmt.Sprintf(`Usage:
  %s [--sleep-interval=<seconds>] [--cri-path=<path>] [--watch-namespace=<namespace>] [--sysfs=<mountpoint>] [--sriov-config-file=<path>]
  %s -h | --help
  Options:
  -h --help                       Show this screen.
  --cri-path=<path>               CRI Endpoint file path to use. [Default: %v]
  --sleep-interval=<seconds>      Time to sleep between updates. [Default: %v]
  --watch-namespace=<namespace>   Namespace to watch pods for. Use "" for all namespaces.
  --sysfs=<mountpoint>            Mount point of the sysfs. [Default: %v]
  --sriov-config-file=<path>      SRIOV device plugin config file path.`,
		ProgramName,
		ProgramName,
		args.CRIEndpointPath,
		args.SleepInterval,
		args.SysfsRoot,
	)

	arguments, _ := docopt.ParseArgs(usage, argv, ProgramName)
	var err error
	// Parse argument values as usable types.
	if ns, ok := arguments["--watch-namespace"].(string); ok {
		args.Namespace = ns
	}
	if path, ok := arguments["--sriov-config-file"].(string); ok {
		args.SRIOVConfigFile = path
	}
	args.SysfsRoot = arguments["--sysfs"].(string)
	args.CRIEndpointPath = arguments["--cri-path"].(string)
	args.SleepInterval, err = time.ParseDuration(arguments["--sleep-interval"].(string))
	if err != nil {
		return args, fmt.Errorf("invalid --sleep-interval specified: %s", err.Error())
	}
	return args, nil
}
