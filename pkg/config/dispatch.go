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
	"strings"

	"github.com/knadh/koanf"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podrescli"
)

func dispatchValues(k *koanf.Koanf, pArgs *ProgArgs) error {
	pArgs.Core.Debug = k.Bool("core.debug")
	pArgs.Core.DumpConfig = k.String("core.dump-config")
	pArgs.Core.Version = k.Bool("core.version")

	pArgs.NRTupdater.NoPublish = k.Bool("updater.no-publish")
	pArgs.NRTupdater.Oneshot = k.Bool("updater.oneshot")
	pArgs.NRTupdater.Hostname = k.String("updater.hostname")

	pArgs.Resourcemonitor.Namespace = k.String("monitor.watch-namespace")
	pArgs.Resourcemonitor.SysfsRoot = k.String("monitor.sysfs")
	pArgs.Resourcemonitor.ExcludeList = k.StringsMap("resource-exclude-list") // note the partial path: intentionally NOT exposed as flag
	pArgs.Resourcemonitor.PodSetFingerprint = k.Bool("monitor.pods-fingerprint")
	pArgs.Resourcemonitor.ExposeTiming = k.Bool("monitor.expose-timing")
	pArgs.Resourcemonitor.RefreshNodeResources = k.Bool("monitor.refresh-node-resources")

	pArgs.RTE.TopologyManagerPolicy = k.String("exporter.topology-manager-policy")
	pArgs.RTE.TopologyManagerScope = k.String("exporter.topology-manager-scope")
	pArgs.RTE.KubeletConfigFile = k.String("exporter.kubelet-config-file")
	pArgs.RTE.PodResourcesSocketPath = k.String("exporter.podresources-socket")
	pArgs.RTE.SleepInterval = k.Duration("exporter.sleep-interval")
	pArgs.RTE.PodReadinessEnable = k.Bool("exporter.podreadiness")
	pArgs.RTE.NotifyFilePath = k.String("exporter.notify-file")
	pArgs.RTE.MaxEventsPerTimeUnit = k.Int64("exporter.max-events-per-second")

	var err error
	pArgs.RTE.KubeletStateDirs, err = setKubeletStateDirs(k.String("exporter.kubelet-state-dir"))
	if err != nil {
		return err
	}

	pArgs.RTE.ReferenceContainer, err = setContainerIdent(k.String("exporter.reference-container"))
	if err != nil {
		return err
	}
	if pArgs.RTE.ReferenceContainer.IsEmpty() {
		pArgs.RTE.ReferenceContainer = podrescli.ContainerIdentFromEnv()
	}

	return nil
}

func setKubeletStateDirs(value string) ([]string, error) {
	if value == "" {
		return nil, nil
	}
	ksd := make([]string, 0)
	for _, s := range strings.Split(value, " ") {
		ksd = append(ksd, strings.TrimSpace(s))
	}
	return ksd, nil
}

func setContainerIdent(value string) (*podrescli.ContainerIdent, error) {
	ci, err := podrescli.ContainerIdentFromString(value)
	if err != nil {
		return nil, err
	}

	if ci == nil {
		return &podrescli.ContainerIdent{}, nil
	}

	return ci, nil
}
