/*
Copyright 2022 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"encoding/json"
	"os"

	"sigs.k8s.io/yaml"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/nrtupdater"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcemonitor"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcetopologyexporter"
)

type GlobalArgs struct {
	KubeConfig string `json:"kubeConfig,omitempty"`
	Debug      bool   `json:"debug,omitempty"`
	Verbose    int    `json:"verbose"`
}

func (args GlobalArgs) Clone() GlobalArgs {
	return GlobalArgs{
		KubeConfig: args.KubeConfig,
		Debug:      args.Debug,
		Verbose:    args.Verbose,
	}
}

type ProgArgs struct {
	Global          GlobalArgs                    `json:"global,omitempty"`
	NRTupdater      nrtupdater.Args               `json:"nrtUpdater,omitempty"`
	Resourcemonitor resourcemonitor.Args          `json:"resourceMonitor,omitempty"`
	RTE             resourcetopologyexporter.Args `json:"topologyExporter,omitempty"`
	Version         bool                          `json:"-"`
	DumpConfig      string                        `json:"-"`
}

func (pa *ProgArgs) ToJson() ([]byte, error) {
	return json.Marshal(pa)
}

func (pa *ProgArgs) ToYaml() ([]byte, error) {
	return yaml.Marshal(pa)
}

func (pa ProgArgs) Clone() ProgArgs {
	return ProgArgs{
		Global:          pa.Global.Clone(),
		NRTupdater:      pa.NRTupdater.Clone(),
		Resourcemonitor: pa.Resourcemonitor.Clone(),
		RTE:             pa.RTE.Clone(),
		Version:         pa.Version,
		DumpConfig:      pa.DumpConfig,
	}
}

// The args is passed only for testing purposes.
func LoadArgs(args ...string) (ProgArgs, error) {
	var err error
	var configRoot string
	var extraConfigPath string
	var pArgs ProgArgs

	SetDefaults(&pArgs)

	configRoot, extraConfigPath, err = FromFlags(&pArgs, args...)

	if pArgs.Version {
		return pArgs, err
	}

	err = FromFiles(&pArgs, configRoot, extraConfigPath)
	if err != nil {
		return pArgs, err
	}

	err = FromEnv(&pArgs)
	if err != nil {
		return pArgs, err
	}

	err = Validate(&pArgs)
	if err != nil {
		return pArgs, err
	}

	err = Finalize(&pArgs)
	return pArgs, err
}

func Finalize(pArgs *ProgArgs) error {
	var err error
	if pArgs.NRTupdater.Hostname == "" {
		pArgs.NRTupdater.Hostname, err = os.Hostname()
	}
	return err
}
