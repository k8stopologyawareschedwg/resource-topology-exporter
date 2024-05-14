/*
Copyright 2024 The Kubernetes Authors.

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

import "testing"

func TestFromEnv(t *testing.T) {
	nodeName := "n1.test.io"
	refNs := "foons"
	refPod := "barpod"
	refCnt := "bazcnt"
	tmPol := "restricted"
	tmScope := "pod"

	closer := setupTestWithEnv(t, map[string]string{
		"TOPOLOGY_MANAGER_POLICY":  tmPol,
		"TOPOLOGY_MANAGER_SCOPE":   tmScope,
		"NODE_NAME":                nodeName,
		"REFERENCE_NAMESPACE":      refNs,
		"REFERENCE_POD_NAME":       refPod,
		"REFERENCE_CONTAINER_NAME": refCnt,
	})
	t.Cleanup(closer)

	var pArgs ProgArgs
	SetDefaults(&pArgs)
	FromEnv(&pArgs)

	if pArgs.Resourceupdater.Hostname != nodeName {
		t.Errorf("hostname mismatch got %q expected %q", pArgs.Resourceupdater.Hostname, nodeName)
	}
	if pArgs.RTE.TopologyManagerPolicy != tmPol {
		t.Errorf("TM policy mismatch got %q expected %q", pArgs.RTE.TopologyManagerPolicy, tmPol)
	}
	if pArgs.RTE.TopologyManagerScope != tmScope {
		t.Errorf("TM scope mismatch got %q expected %q", pArgs.RTE.TopologyManagerScope, tmScope)
	}
}
