/*
Copyright 2021 The Kubernetes Authors.

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

package testenv

import (
	"os"
)

const (
	// we rely on kind for our CI
	DefaultNodeName             = "kind-worker"
	DefaultNamespace            = ""
	DefaultTopologyMangerPolicy = "none"
	DefaultRTEPollInterval      = "10s"
	RTELabelName                = "resource-topology"
	RTEContainerName            = "resource-topology-exporter-container"
)

var (
	currentNodeName       string
	currentNamespace      string
	currentTopologyPolicy string
)

func init() {
	currentNodeName = DefaultNodeName
	currentNamespace = DefaultNamespace
	currentTopologyPolicy = DefaultTopologyMangerPolicy
}

func GetNodeName() string {
	if nodeName, ok := os.LookupEnv("E2E_WORKER_NODE_NAME"); ok {
		return nodeName
	}
	return currentNodeName
}

func GetNamespaceName() string {
	if nsName, ok := os.LookupEnv("E2E_NAMESPACE_NAME"); ok {
		return nsName
	}
	return currentNamespace
}

func GetTopologyManagerPolicy() string {
	if tmPolicy, ok := os.LookupEnv("E2E_TOPOLOGY_MANAGER_POLICY"); ok {
		return tmPolicy
	}
	return currentTopologyPolicy
}

func GetPollInterval() string {
	pollInterval, ok := os.LookupEnv("RTE_POLL_INTERVAL")
	if !ok {
		// nothing to do!
		return DefaultRTEPollInterval
	}
	return pollInterval
}

func SetNodeName(nodeName string) {
	currentNodeName = nodeName
}

func SetNamespaceName(namespaceName string) {
	currentNamespace = namespaceName
}
