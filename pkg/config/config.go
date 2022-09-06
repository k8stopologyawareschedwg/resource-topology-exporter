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
	goflag "flag"
	"time"

	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"

	"github.com/knadh/koanf"
	koanfyaml "github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/nrtupdater"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcemonitor"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcetopologyexporter"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/version"
)

const (
	keyDelim string = "."
)

type CoreArgs struct {
	Debug      bool   `koanf:"debug" json:"debug"`
	DumpConfig string `koanf:"dump-config" json:"dump-config"`
	Version    bool   `koanf:"version" json:"version"`
}

type ProgArgs struct {
	Core            CoreArgs                      `koanf:"core" json:"core"`
	NRTupdater      nrtupdater.Args               `koanf:"updater" json:"updater"`
	Resourcemonitor resourcemonitor.Args          `koanf:"monitor" json:"monitor"`
	RTE             resourcetopologyexporter.Args `koanf:"exporter" json:"exporter"`
}

func (pa *ProgArgs) ToJson() ([]byte, error) {
	return json.Marshal(pa)
}

func (pa *ProgArgs) ToYaml() ([]byte, error) {
	return yaml.Marshal(pa)
}

// The args is passed only for testing purposes.
func LoadArgs(args ...string) (ProgArgs, error) {
	pArgs := ProgArgs{
		RTE: resourcetopologyexporter.Args{
			TimeUnitToLimitEvents: time.Second,
		},
	}

	cmdline := goflag.NewFlagSet(version.ProgramName, goflag.ExitOnError)
	klog.InitFlags(cmdline)

	flags, cfgFiles, keyMapper := setupFlags()
	flags.AddGoFlagSet(cmdline)

	err := flags.Parse(args)
	if err != nil {
		return pArgs, err
	}

	if pArgs.Core.Version {
		return pArgs, err
	}

	k := koanf.New(keyDelim)

	// Load the config files provided in the commandline.
	for _, cfgFile := range splitConfigFiles(cfgFiles) {
		klog.V(4).Infof("reading config file: %q", cfgFile)
		if err := k.Load(file.Provider(cfgFile), koanfyaml.Parser()); err != nil {
			klog.Infof("ignoring config file %q - (error loading file: %v)", cfgFile, err)
		}
	}

	if err := k.Load(posflag.ProviderWithValue(flags, ".", k, keyMapper), nil); err != nil {
		klog.Fatalf("error loading config: %v", err)
	}

	err = dispatchValues(k, &pArgs)
	if err != nil {
		return pArgs, err
	}
	return pArgs, nil
}
