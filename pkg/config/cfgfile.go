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
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"

	flatten "github.com/jeremywohl/flatten/v2"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres/middleware/podexclude"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcemonitor"
)

const (
	DefaultconfigRoot     = "/etc/rte"
	LegacyExtraConfigPath = "/etc/resource-topology-exporter/config.yaml"

	configDirDaemon = "daemon"
	configDirExtra  = "extra"
)

func FixExtraConfigPath(configRoot string) string {
	return filepath.Join(configRoot, configDirExtra, "config.yaml")
}

func FromFiles(pArgs *ProgArgs, configRoot, extraConfigPath string) error {
	if configRoot == "" {
		return errors.New("configRoot is not allowed to be an empty string")
	}
	if extraConfigPath == "" {
		return errors.New("extraConfigPath is not allowed to be an empty string")
	}
	err := fromDaemonFiles(pArgs, configRoot)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return fromExtraFile(pArgs, extraConfigPath)
}

func fromExtraFile(pArgs *ProgArgs, extraConfigPath string) error {
	conf, err := readExtraConfig(extraConfigPath)
	if err != nil {
		return fmt.Errorf("error getting exclude list from the configuration: %w", err)
	}

	if pArgs.Global.Debug {
		klog.Infof("configfile extra data: %+v", conf)
	}

	return setupArgsFromConfig(pArgs, conf)

}

func fromDaemonFiles(pArgs *ProgArgs, configPathRoot string) error {
	// the configuration processing flow could be summarized as applying a series
	// of patches from various user-provided sources (files, envs, flags...) over
	// the builtin defaults.
	// Handling configuration files (main + configlets) is implemented using the same approach.
	// We need to overcome two hurdles, which both stems from how golang treats the zero values,
	// which is fine in general but only annopying in this specific corner case.
	// 1. we need to distinguish "value not set" from "value with defaults" (language's or our's)
	//    To do so, using the simple unmashalling is not good enough, because unset values
	//    will be set to zero values. But we want to LACK unset values, so we know we don't
	//    apply over the previos iteration, keeping the oldest set value (possibly the default)
	// 2. hence, we don't use the simple unmarshalling, but we unmarshal to generic maps
	//    unmarshalled maps will be map[[string]any, possibly nested. But at least we will have
	//    decoded only the value explicitely given, which makes simple to know when to apply
	//    a new value and when keep the old value

	confObj := make(map[string]interface{})

	configPath := filepath.Join(configPathRoot, "daemon", "config.yaml")
	if pArgs.Global.Debug {
		klog.Infof("loading configlet: %q", configPath)
	}
	err := loadConfiglet(confObj, configPath)
	if err != nil {
		return err
	}

	// this directory may be missing, that's expected and fine
	configletDir := filepath.Join(configPathRoot, "daemon", "config.yaml.d")
	if configlets, err := ReadConfigletDir(configletDir); err == nil {
		for _, configlet := range configlets {
			if !configlet.Type().IsRegular() {
				klog.Infof("configlet %q not regular file: ignored", configlet.Name())
				continue
			}
			configletPath := filepath.Join(configletDir, configlet.Name())
			if pArgs.Global.Debug {
				klog.Infof("loading configlet: %q", configletPath)
			}
			err = loadConfiglet(confObj, configletPath)
			if err != nil {
				return err
			}
		}
	}

	if pArgs.Global.Debug {
		klog.Infof("configfile daemon data: %+v", confObj)
	}
	return dispatchConfObj(confObj, pArgs)
}

func loadConfiglet(confObj map[string]interface{}, configPath string) error {
	data, err := ReadConfiglet(configPath)
	if err != nil {
		return err
	}
	obj := make(map[string]interface{})
	err = yaml.Unmarshal(data, &obj)
	if err != nil {
		return err
	}

	// we can read arbitrarly nested structs (decoded as map[string]any), which
	// will complicate the merge logic due to the need to navigated the nested
	// maps. But we can leverage the fact that if we consider the nested maps
	// as N-trees, every path from the root to leaves - actual config values
	// will be unique by design, and that the tree will be very shallow.
	// we expect something like
	// /
	// +- global
	// |  +- debug
	// +- resourceMonitor
	// |  +- podSetFingerprint: true
	// +- topologyExporter
	//    +- referenceContainer
	//       +- ContainerName: "foo"
	// ...
	// so we will have something like
	// refCnt := conf["topologyExporter"]["referenceContainer"]["ContainerName"]
	// (note this very specific example is however treated differently)
	//
	// if we flatten the layout to a simple map, we will have a trivial merge logic
	// which won't need to navigate the nested maps:
	//
	// refCnt := conf["topologyExporter.referenceContainer.ContainerName"]
	//
	// to flatten the maps, we use the flatten package from 3rd party. It exposes
	// a couple functions, it has a very permissive license and it seems a good
	// tradeoff for this specific functionality.
	// NOTE: this approach is inspired from what the koanf package does
	//       https://github.com/knadh/koanf
	flat, err := flatten.Flatten(obj, "", flatten.DotStyle)
	if err != nil {
		return err
	}

	mergeConfObj(confObj, flat)
	return nil
}

type kubeletParams struct {
	TopologyManagerPolicy string `json:"topologyManagerPolicy,omitempty"`
	TopologyManagerScope  string `json:"topologyManagerScope,omitempty"`
}

type config struct {
	Kubelet         kubeletParams                   `json:"kubelet,omitempty"`
	ResourceExclude resourcemonitor.ResourceExclude `json:"resourceExclude,omitempty"`
	PodExclude      podexclude.List                 `json:"podExclude,omitempty"`
}

func readExtraConfig(configPath string) (config, error) {
	conf := config{}
	data, err := ReadConfiglet(configPath)
	if err != nil {
		// config is optional
		if errors.Is(err, os.ErrNotExist) {
			klog.Infof("couldn't find configuration in %q", configPath)
			return conf, nil
		}
		return conf, err
	}
	err = yaml.Unmarshal(data, &conf)
	return conf, err
}

func setupArgsFromConfig(pArgs *ProgArgs, conf config) error {
	if len(conf.ResourceExclude) > 0 {
		pArgs.Resourcemonitor.ResourceExclude = conf.ResourceExclude
		klog.V(2).Infof("using resources exclude:\n%s", pArgs.Resourcemonitor.ResourceExclude.String())
	}

	if len(conf.PodExclude) > 0 {
		pArgs.Resourcemonitor.PodExclude = conf.PodExclude
		klog.V(2).Infof("using pod excludes:\n%s", pArgs.Resourcemonitor.PodExclude.String())
	}

	if pArgs.RTE.TopologyManagerPolicy == "" {
		pArgs.RTE.TopologyManagerPolicy = conf.Kubelet.TopologyManagerPolicy
		klog.V(2).Infof("using kubelet topology manager policy: %q", pArgs.RTE.TopologyManagerPolicy)
	}
	if pArgs.RTE.TopologyManagerScope == "" {
		pArgs.RTE.TopologyManagerScope = conf.Kubelet.TopologyManagerScope
		klog.V(2).Infof("using kubelet topology manager scope: %q", pArgs.RTE.TopologyManagerScope)
	}

	return nil
}

func mergeConfObj(obj, upd map[string]interface{}) {
	for key, val := range upd {
		obj[key] = val
	}
}
