package main

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/docopt/docopt-go"
	"github.com/swatisehgal/resource-topology-exporter/pkg/exporter"
	"github.com/swatisehgal/resource-topology-exporter/pkg/finder"
	"k8s.io/api/core/v1"
	"log"
	"time"
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

	// Get new finder instance
	instance, err := finder.NewFinder(args)
	if err != nil {
		log.Fatalf("Failed to initialize NfdWorker instance: %v", err)
	}

	for {
		if err = instance.Run(); err != nil {
			log.Fatalf("ERROR: %v", err)
		}

		allocatedCpusNumaInfo := instance.GetAllocatedCPUs()
		log.Printf("allocatedCpusNumaInfo:%v", allocatedCpusNumaInfo)
		allocatedResourcesNumaInfo := instance.GetAllocatedDevices()

		for _, allocatedResourceNumaInfo := range allocatedResourcesNumaInfo {
			for _, cpuNUMANodeResource := range allocatedCpusNumaInfo {
				if cpuNUMANodeResource.NUMAID == allocatedResourceNumaInfo.NUMAID {
					allocatedResourceNumaInfo.Resources[v1.ResourceName("cpu")] = cpuNUMANodeResource.Resources[v1.ResourceName("cpu")]
				}

			}
		}
		log.Printf("allocatedResourcesNumaInfo:%v", spew.Sdump(allocatedResourcesNumaInfo))

		crdExporter := exporter.NewExporter(allocatedResourcesNumaInfo)
		if err = crdExporter.Run(); err != nil {
			log.Fatalf("ERROR: %v", err)
		}

		time.Sleep(args.SleepInterval)
	}
}

// argsParse parses the command line arguments passed to the program.
// The argument argv is passed only for testing purposes.
func argsParse(argv []string) (finder.Args, error) {
	args := finder.Args{}
	usage := fmt.Sprintf(`%s.
  Usage:
  %s [--sleep-interval=<seconds>] [--cri-path=<path>]
  %s -h | --help
  Options:
  -h --help                   Show this screen.
  --cri-path=<path>           CRI Enddpoint file path to use.
                              [Default: /host-run/containerd/containerd.sock]
  --sleep-interval=<seconds>  Time to sleep between updates. [Default: 3s]`,
		ProgramName,
		ProgramName,
		ProgramName,
	)

	arguments, _ := docopt.ParseArgs(usage, argv, fmt.Sprintf("%s", ProgramName))
	var err error
	// Parse argument values as usable types.
	args.CRIEndpointPath = arguments["--cri-path"].(string)
	args.SleepInterval, err = time.ParseDuration(arguments["--sleep-interval"].(string))
	if err != nil {
		return args, fmt.Errorf("invalid --sleep-interval specified: %s", err.Error())
	}
	return args, nil
}
