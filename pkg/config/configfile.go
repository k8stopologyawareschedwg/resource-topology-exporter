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

	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres/middleware/podexclude"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcemonitor"
)

func FromFiles(pArgs *ProgArgs, configPath string) error {
	conf, err := readExtraConfig(configPath)
	if err != nil {
		return fmt.Errorf("error getting exclude list from the configuration: %w", err)
	}
	return setupArgsFromConfig(pArgs, conf)
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
	data, err := os.ReadFile(configPath)
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
