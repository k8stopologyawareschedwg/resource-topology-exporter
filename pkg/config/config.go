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
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/kloglevel"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/nrtupdater"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcemonitor"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcetopologyexporter"
)

var (
	SkipDirectory = errors.New("skip config directory")
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

func (pa *ProgArgs) ToJSONString() string {
	data, err := json.Marshal(pa)
	if err != nil {
		return fmt.Sprintf("<ERROR=%q>", err)
	}
	return string(data)
}

func (pa *ProgArgs) ToYAMLString() string {
	data, err := yaml.Marshal(pa)
	if err != nil {
		return fmt.Sprintf("<ERROR=%q>", err)
	}
	return string(data)
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
	tmp := pArgs.Clone()

	// read first the flags, and discard everything. We need to do this
	// to learn about the config root, and the most robust way (bar only)
	// to learn about the parameters is using flags.Parse(), so be it.
	// the action will waste few cycles but is expected to be idempotent.
	configRoot, extraConfigPath, err = FromFlags(&tmp, args...)
	if err != nil {
		return pArgs, err
	}
	// this is needed to keep the priority from flags, because otherwise
	// a default in the config file may surprisingly reset the debug value.
	pArgs.Global.Debug = tmp.Global.Debug
	pArgs.Version = tmp.Version
	pArgs.DumpConfig = tmp.DumpConfig

	if pArgs.Version {
		return pArgs, err
	}

	// now the real processing begins. From now on we waste nothing
	if pArgs.Global.Debug {
		klog.Infof("configRoot=%q extraConfigPath=%q", configRoot, extraConfigPath)
		klog.Infof("config defaults:{{\n%s}}", pArgs.ToYAMLString())
	}

	err = FromFiles(&pArgs, configRoot, extraConfigPath)
	if err != nil {
		return pArgs, err
	}
	if pArgs.Global.Debug {
		klog.Infof("config from configuration files:{{\n%s}}", pArgs.ToYAMLString())
	}

	FromEnv(&pArgs)
	if pArgs.Global.Debug {
		klog.Infof("config from environment variables:{{\n%s}}", pArgs.ToYAMLString())
	}

	_, _, err = FromFlags(&pArgs, args...)
	if err != nil {
		return pArgs, err
	}
	if pArgs.Global.Debug {
		klog.Infof("config from flags:{{\n%s\n}}", pArgs.ToYAMLString())
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

	// sync back klog settings
	kloglevel.Set(CommandLine, klog.Level(pArgs.Global.Verbose)) // for consistency
	if pArgs.Global.Debug {
		VL, err := kloglevel.Get(CommandLine)
		if err != nil {
			klog.Errorf("cannot get back klog level: %v", err)
		} else {
			klog.Infof("klog V=%d", VL)
		}
	}

	return err
}

func UserRunDir() (string, error) {
	return filepath.Join("/run", "user", fmt.Sprintf("%d", os.Getuid()), "rte"), nil
}

func UserHomeDir() (string, error) {
	homeDir, ok := os.LookupEnv("HOME")
	if !ok || homeDir == "" {
		// can happen in CI
		return "", SkipDirectory
	}
	return filepath.Join(homeDir, ".rte"), nil
}
