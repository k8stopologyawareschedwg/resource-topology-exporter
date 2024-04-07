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
	"time"

	"github.com/k8stopologyawareschedwg/podfingerprint"
	metricssrv "github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/metrics/server"
)

const (
	DefaultConfigRoot = "/etc/rte"
)

func SetDefaults(pArgs *ProgArgs) {
	pArgs.Global.Verbose = 2
	pArgs.Resourcemonitor.SysfsRoot = "/sys"
	pArgs.Resourcemonitor.PodSetFingerprint = true
	pArgs.Resourcemonitor.PodSetFingerprintMethod = podfingerprint.MethodWithExclusiveResources
	pArgs.RTE.SleepInterval = 60 * time.Second
	pArgs.RTE.KubeletConfigFile = "/podresources/config.yaml"
	pArgs.RTE.PodResourcesSocketPath = "unix:///podresources/kubelet.sock"
	pArgs.RTE.PodReadinessEnable = true
	pArgs.RTE.AddNRTOwnerEnable = true
	pArgs.RTE.MetricsMode = metricssrv.ServingDefault
	pArgs.RTE.MetricsPort = metricssrv.PortDefault
	pArgs.RTE.MetricsAddress = metricssrv.AddressDefault
	pArgs.RTE.MetricsTLSCfg = metricssrv.NewDefaultTLSConfig()
	pArgs.RTE.MaxEventsPerTimeUnit = 1
	pArgs.RTE.TimeUnitToLimitEvents = time.Second
}
