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
	"os"

	metricssrv "github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/metrics/server"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres/middleware/sharedcpuspool"
)

func HostNameFromEnv() string {
	if val, ok := os.LookupEnv("NODE_NAME"); ok {
		return val
	}
	return ""
}

func TopologyManagerPolicyFromEnv() string {
	if val, ok := os.LookupEnv("TOPOLOGY_MANAGER_POLICY"); ok {
		return val
	}
	// empty string is a valid value here, so just keep going
	return ""
}

func TopologyManagerScopeFromEnv() string {
	if val, ok := os.LookupEnv("TOPOLOGY_MANAGER_SCOPE"); ok {
		return val
	}
	// empty string is a valid value here, so just keep going
	return ""
}

func FromEnv(pArgs *ProgArgs) {
	if pArgs.NRTupdater.Hostname == "" {
		pArgs.NRTupdater.Hostname = HostNameFromEnv()
	}
	if pArgs.RTE.TopologyManagerPolicy == "" {
		pArgs.RTE.TopologyManagerPolicy = TopologyManagerPolicyFromEnv()
	}
	if pArgs.RTE.TopologyManagerScope == "" {
		pArgs.RTE.TopologyManagerScope = TopologyManagerScopeFromEnv()
	}
	if pArgs.RTE.ReferenceContainer.IsEmpty() {
		pArgs.RTE.ReferenceContainer = sharedcpuspool.ContainerIdentFromEnv()
	}
	if pArgs.RTE.MetricsPort == 0 {
		pArgs.RTE.MetricsPort = metricssrv.PortFromEnv()
	}
	if pArgs.RTE.MetricsAddress == "" {
		pArgs.RTE.MetricsAddress = metricssrv.AddressFromEnv()
	}
}
