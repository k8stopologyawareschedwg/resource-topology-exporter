package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"text/template"
	"time"

	"github.com/docopt/docopt-go"

	"sigs.k8s.io/yaml"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/nrtupdater"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podrescli"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/prometheus"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcemonitor"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcetopologyexporter"
)

const (
	// ProgramName is the canonical name of this program
	ProgramName = "resource-topology-exporter"
)

func main() {
	nrtupdaterArgs, resourcemonitorArgs, rteArgs, err := argsParse(os.Args[1:])
	if err != nil {
		log.Fatalf("failed to parse command line: %v", err)
	}

	cli, err := podrescli.NewFilteringClient(resourcemonitorArgs.PodResourceSocketPath, rteArgs.Debug, rteArgs.ReferenceContainer)
	if err != nil {
		log.Fatalf("failed to get podresources client: %v", err)
	}

	err = prometheus.InitPrometheus()
	if err != nil {
		log.Fatalf("failed to start prometheus server: %v", err)
	}

	err = resourcetopologyexporter.Execute(cli, nrtupdaterArgs, resourcemonitorArgs, rteArgs)
	if err != nil {
		log.Fatalf("failed to execute: %v", err)
	}
}

const helpTemplate string = `{{.ProgramName}}

  Usage:
  {{.ProgramName}}	[--debug]
                        [--no-publish]
			[--oneshot | --sleep-interval=<seconds>]
			[--podresources-socket=<path>]
			[--export-namespace=<namespace>]
			[--watch-namespace=<namespace>]
			[--sysfs=<mountpoint>]
			[--kubelet-state-dir=<path>...]
			[--kubelet-config-file=<path>]
			[--topology-manager-policy=<pol>]
			[--reference-container=<spec>]
			[--config=<path>]
			[--refresh-allocatable]

  {{.ProgramName}} -h | --help
  {{.ProgramName}} --version

  Options:
  -h --help                       Show this screen.
  --debug                         Enable debug output. [Default: false]
  --version                       Output version and exit.
  --no-publish                    Do not publish discovered features to the
                                  cluster-local Kubernetes API server.
  --hostname                      Override the node hostname.
  --oneshot                       Update once and exit.
  --sleep-interval=<seconds>      Time to sleep between podresources API polls.
                                  [Default: 60s]
  --export-namespace=<namespace>  Namespace on which update CRDs. Use "" for all namespaces.
  --watch-namespace=<namespace>   Namespace to watch pods for. Use "" for all namespaces.
  --sysfs=<path>                  Top-level component path of sysfs. [Default: /sys]
  --kubelet-config-file=<path>    Kubelet config file path. [Default: ]
  --topology-manager-policy=<pol> Explicitely set the topology manager policy instead of reading
                                  from the kubelet. [Default: ]
  --kubelet-state-dir=<path>...   Kubelet state directory (RO access needed), for smart polling.
  --podresources-socket=<path>    Pod Resource Socket path to use.
                                  [Default: unix:///podresources/kubelet.sock]
  --reference-container=<spec>    Reference container, used to learn about the shared cpu pool
                                  See: https://github.com/kubernetes/kubernetes/issues/102190
                                  format of spec is namespace/podname/containername.
                                  Alternatively, you can use the env vars
                                  REFERENCE_NAMESPACE, REFERENCE_POD_NAME, REFERENCE_CONTAINER_NAME.
  --config=<path>                 Configuration file path. Use this to set the exclude list.
                                  [Default: /etc/resource-topology-exporter/config.yaml]
  --refresh-allocatable           Refresh allocatable resources before each poll.`

func getUsage() (string, error) {
	var helpBuffer bytes.Buffer
	helpData := struct {
		ProgramName string
	}{
		ProgramName: ProgramName,
	}

	tmpl, err := template.New("help").Parse(helpTemplate)
	if err != nil {
		return "", err
	}
	err = tmpl.Execute(&helpBuffer, helpData)
	if err != nil {
		return "", err
	}

	return helpBuffer.String(), nil
}

// nrtupdaterArgsParse parses the command line arguments passed to the program.
// The argument argv is passed only for testing purposes.
func argsParse(argv []string) (nrtupdater.Args, resourcemonitor.Args, resourcetopologyexporter.Args, error) {
	var nrtupdaterArgs nrtupdater.Args
	var resourcemonitorArgs resourcemonitor.Args
	var rteArgs resourcetopologyexporter.Args

	usage, err := getUsage()
	if err != nil {
		return nrtupdaterArgs, resourcemonitorArgs, rteArgs, err
	}

	arguments, _ := docopt.ParseArgs(usage, argv, fmt.Sprintf("%s %s", ProgramName, "TBD"))

	// Parse argument values as usable types.
	nrtupdaterArgs.NoPublish = arguments["--no-publish"].(bool)
	nrtupdaterArgs.Oneshot = arguments["--oneshot"].(bool)
	if ns, ok := arguments["--export-namespace"].(string); ok {
		nrtupdaterArgs.Namespace = ns
	}
	if hostname, ok := arguments["--hostname"].(string); ok {
		nrtupdaterArgs.Hostname = hostname
	}
	if nrtupdaterArgs.Hostname == "" {
		var err error
		nrtupdaterArgs.Hostname = os.Getenv("NODE_NAME")
		if nrtupdaterArgs.Hostname == "" {
			nrtupdaterArgs.Hostname, err = os.Hostname()
			if err != nil {
				return nrtupdaterArgs, resourcemonitorArgs, rteArgs, fmt.Errorf("error getting the host name: %w", err)
			}
		}
	}

	resourcemonitorArgs.SleepInterval, err = time.ParseDuration(arguments["--sleep-interval"].(string))
	if err != nil {
		return nrtupdaterArgs, resourcemonitorArgs, rteArgs, fmt.Errorf("invalid --sleep-interval specified: %w", err)
	}
	if ns, ok := arguments["--watch-namespace"].(string); ok {
		resourcemonitorArgs.Namespace = ns
	}
	if kubeletConfigPath, ok := arguments["--kubelet-config-file"].(string); ok {
		resourcemonitorArgs.KubeletConfigFile = kubeletConfigPath
	}
	resourcemonitorArgs.SysfsRoot = arguments["--sysfs"].(string)
	resourcemonitorArgs.RefreshAllocatable = arguments["--refresh-allocatable"].(bool)

	rteArgs.Debug = arguments["--debug"].(bool)
	if refCnt, ok := arguments["--reference-container"].(string); ok {
		rteArgs.ReferenceContainer, err = resourcetopologyexporter.ContainerIdentFromString(refCnt)
		if err != nil {
			return nrtupdaterArgs, resourcemonitorArgs, rteArgs, err
		}
	}
	if rteArgs.ReferenceContainer == nil {
		rteArgs.ReferenceContainer = resourcetopologyexporter.ContainerIdentFromEnv()
	}

	if configPath, ok := arguments["--config"].(string); ok {
		conf, err := readConfig(configPath)
		if err != nil {
			log.Fatalf("error getting exclude list from the configuration: %v", err)
		}
		resourcemonitorArgs.ExcludeList.ExcludeList = conf.ExcludeList
		log.Printf("using exclude list:\n%s", resourcemonitorArgs.ExcludeList.String())
	}
	if tmPolicy, ok := arguments["--topology-manager-policy"].(string); ok {
		if tmPolicy == "" {
			// last attempt
			tmPolicy = os.Getenv("TOPOLOGY_MANAGER_POLICY")
		}
		// empty string is a valid value here, so just keep going
		rteArgs.TopologyManagerPolicy = tmPolicy
	}
	return nrtupdaterArgs, resourcemonitorArgs, rteArgs, nil
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
			log.Printf("Info: couldn't find configuration in %q", configPath)
			return conf, nil
		}
		return conf, err
	}
	err = yaml.Unmarshal(data, &conf)
	return conf, err
}
