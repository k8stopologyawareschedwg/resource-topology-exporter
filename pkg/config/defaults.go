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

	"k8s.io/klog/v2"
)

func DefaultHostName() string {
	var err error

	val, ok := os.LookupEnv("NODE_NAME")
	if !ok || val == "" {
		val, err = os.Hostname()
		if err != nil {
			klog.Fatalf("error getting the host name: %v", err)
		}
	}
	return val
}

func DefaultTopologyManagerPolicy() string {
	if val, ok := os.LookupEnv("TOPOLOGY_MANAGER_POLICY"); ok {
		return val
	}
	// empty string is a valid value here, so just keep going
	return ""
}

func DefaultTopologyManagerScope() string {
	if val, ok := os.LookupEnv("TOPOLOGY_MANAGER_SCOPE"); ok {
		return val
	}
	// empty string is a valid value here, so just keep going
	return ""
}
