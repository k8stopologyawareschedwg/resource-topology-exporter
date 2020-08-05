package main

import (
	"fmt"
	"log"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/docopt/docopt-go"

	"github.com/swatisehgal/resource-topology-exporter/pkg/exporter"
	"github.com/swatisehgal/resource-topology-exporter/pkg/finder"
	"github.com/swatisehgal/resource-topology-exporter/pkg/kubeconf"
	"github.com/swatisehgal/resource-topology-exporter/pkg/pciconf"
)

const (
	// ProgramName is the canonical name of this program
	ProgramName       = "resource-topology-exporter"
	ContainerdRuntime = "containerd"
	CRIORuntime       = "cri-o"
	CRISource         = "cri"
	PodResourceSource = "pod-resource-api"
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
	klConfig, err := kubeconf.GetKubeletConfigFromLocalFile(args.KubeletConfigFile)
	if err != nil {
		log.Fatalf("error getting topology Manager Policy: %v", err)
	}
	tmPolicy := klConfig.TopologyManagerPolicy
	log.Printf("Detected kubelet Topology Manager policy %q", tmPolicy)

	var pci2ResMap pciconf.PCIResourceMap
	log.Printf("getting SRIOV configuration from file: %s", args.SRIOVConfigFile)
	pci2ResMap, err = pciconf.GetFromSRIOVConfigFile(args.SysfsRoot, args.SRIOVConfigFile)
	if err != nil {
		log.Fatalf("failed to read the PCI -> Resource mapping: %v", err)
	}

	// Get new finder instance
	var finderInstance finder.Finder
	if args.Source == CRISource {
		finderInstance, err = finder.NewCRIFinder(args, pci2ResMap)
	} else {
		//args.Source == PodResourceSource
		finderInstance, err = finder.NewPodResourceFinder(args, pci2ResMap)
	}
	if err != nil {
		log.Fatalf("Failed to initialize Finder instance: %v", err)
	}

	crdExporter, err := exporter.NewExporter(tmPolicy)
	if err != nil {
		log.Fatalf("Failed to initialize crdExporter instance: %v", err)
	}

	// CAUTION: these resources are expected to change rarely - if ever. So we are intentionally do this once during the process lifecycle.
	nodeResourceData, err := finder.NewNodeResources(args.SysfsRoot, pci2ResMap)
	if err != nil {
		log.Fatalf("Failed to obtain node resource information: %v", err)
	}
	for {
		podResources, err := finderInstance.Scan(nodeResourceData.GetPCI2ResourceMap())
		if err != nil {
			log.Printf("Scan failed: %v\n", err)
			continue
		}

		perNumaResources := finder.Aggregate(podResources, nodeResourceData)
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
		Source:            "pod-resource-api",
		SleepInterval:     time.Duration(3 * time.Second),
		SysfsRoot:         "/host-sys",
		SRIOVConfigFile:   "/etc/sriov-config/config.json",
		KubeletConfigFile: "/host-etc/kubernetes/kubelet.conf",
	}
	usage := fmt.Sprintf(`Usage:
  %s [--sleep-interval=<seconds>] [--source=<path>] [--container-runtime=<runtime>] [--cri-socket=<path>] [--podresources-socket=<path>] [--watch-namespace=<namespace>] [--sysfs=<mountpoint>] [--sriov-config-file=<path>] [--kubelet-config-file=<path>]
  %s -h | --help
  Options:
  -h --help                       Show this screen.
	--source=<source>								Evaluation source to be used (pod-resource-api|cri). [Default: %v]
  --container-runtime=<runtime>   Container Runtime to be used (containerd|cri-o).
  --cri-socket=<path>             CRI Socket path to use.
	--podresources-socket=<path>    Pod Resource Socket path to use.
  --sleep-interval=<seconds>      Time to sleep between updates. [Default: %v]
  --watch-namespace=<namespace>   Namespace to watch pods for. Use "" for all namespaces.
  --sysfs=<mountpoint>            Mount point of the sysfs. [Default: %v]
  --sriov-config-file=<path>      SRIOV device plugin config file path. [Default: %v]
  --kubelet-config-file=<path>    Kubelet config file path. [Default: %v]`,
		ProgramName,
		ProgramName,
		args.Source,
		args.SleepInterval,
		args.SysfsRoot,
		args.SRIOVConfigFile,
		args.KubeletConfigFile,
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
	if kubeletConfigPath, ok := arguments["--kubelet-config-file"].(string); ok {
		args.KubeletConfigFile = kubeletConfigPath
	}
	args.SysfsRoot = arguments["--sysfs"].(string)

	if source, ok := arguments["--source"].(string); ok {
		args.Source = source
	}
	if args.Source != PodResourceSource && args.Source != CRISource {
		return args, fmt.Errorf("invalid --source specified")

	} else if args.Source == PodResourceSource {
		//podresource source
		if path, ok := arguments["--podresources-socket"].(string); ok {
			args.PodResourceSocketPath = path
		}
		//return error in case cri-socket path is specified in case of pod-resource-socket source
		if _, ok := arguments["--cri-socket"].(string); ok {
			return args, fmt.Errorf("No need to specify CRI socket path in case pod-resource-api is specified as the source")
		}
		//return error in case container-runtime is specified in case of pod-resource-socket source
		if _, ok := arguments["--container-runtime"].(string); ok {
			return args, fmt.Errorf("No need to specify container runtime in case pod-resource-api is specified as the source")
		}
	} else {
		//cri source
		if path, ok := arguments["--cri-socket"].(string); ok {
			args.CRISocketPath = path
		}
		runtime := arguments["--container-runtime"].(string)
		if runtime != ContainerdRuntime && runtime != CRIORuntime {
			return args, fmt.Errorf("invalid --container-runtime specified")
		}
		args.ContainerRuntime = runtime
		//return error in case pod-resource-socket path is specified in case of cri source
		if _, ok := arguments["--podresources-socket"].(string); ok {
			return args, fmt.Errorf("No need to specify Pod Resource socket path in case CRI is specified as the source")
		}
	}

	args.SleepInterval, err = time.ParseDuration(arguments["--sleep-interval"].(string))
	if err != nil {
		return args, fmt.Errorf("invalid --sleep-interval specified: %s", err.Error())
	}
	return args, nil
}
