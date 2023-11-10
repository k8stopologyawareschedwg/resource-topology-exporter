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
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres/middleware/sharedcpuspool"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcemonitor"
)

const (
	node      = "TEST_NODE"
	namespace = "TEST_NS"
	pod       = "TEST_POD"
	container = "TEST_CONT"
)

var (
	update = flag.Bool("update", false, "update golden files")

	baseDir     string
	testDataDir string
)

func TestOneshot(t *testing.T) {
	closer := setupTest(t)
	t.Cleanup(closer)

	pArgs, err := LoadArgs("--oneshot", "--no-publish")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !pArgs.NRTupdater.Oneshot {
		t.Errorf("oneshot should be true")
	}
	if !pArgs.NRTupdater.NoPublish {
		t.Errorf("nopublish should be true")
	}
}

func TestReferenceContainer(t *testing.T) {
	closer := setupTest(t)
	t.Cleanup(closer)

	pArgs, err := LoadArgs("--reference-container=ns/pod/cont")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedRefCnt := sharedcpuspool.ContainerIdent{Namespace: "ns", PodName: "pod", ContainerName: "cont"}
	if pArgs.RTE.ReferenceContainer.String() != expectedRefCnt.String() {
		t.Errorf("invalid data, got %v expected %v", pArgs.RTE.ReferenceContainer, expectedRefCnt)
	}
}

func TestRefreshNodeResources(t *testing.T) {
	closer := setupTest(t)
	t.Cleanup(closer)

	pArgs, err := LoadArgs("--refresh-node-resources")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !pArgs.Resourcemonitor.RefreshNodeResources {
		t.Errorf("refresh node resources not enabled")
	}
}

func TestDefaults(t *testing.T) {
	closer := setupTest(t)
	t.Cleanup(closer)

	pArgs, err := LoadArgs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pArgsAsJson, err := pArgs.ToJson()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	golden := filepath.Join(testDataDir, fmt.Sprintf("%s.expected.json", t.Name()))
	if *update {
		err = os.WriteFile(golden, pArgsAsJson, 0644)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	expectedAsJson, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// stray newlines make the failure unnecessarily hard to debug
	got := strings.TrimSpace(string(pArgsAsJson))
	expected := strings.TrimSpace(string(expectedAsJson))
	if got != expected {
		t.Errorf("invalid defaults.\n>>> got:\n{%s}\n>>> expected:\n{%s}", got, expected)
	}
}

func TestReadResourceExclude(t *testing.T) {
	closer := setupTest(t)
	t.Cleanup(closer)

	cfg, err := os.CreateTemp("", "exclude-list")
	if err != nil {
		t.Fatalf("unexpected error creating temp file: %v", err)
	}
	t.Cleanup(func() {
		os.Remove(cfg.Name())
	})

	cfgContent := `resourceExclude:
  masternode: [memory, device/exampleA]
  workernode1: [memory, device/exampleB]
  workernode2: [cpu]
  "*": [device/exampleC]`

	if _, err := cfg.Write([]byte(cfgContent)); err != nil {
		t.Fatalf("unexpected error writing data: %v", err)
	}
	if err := cfg.Close(); err != nil {
		t.Fatalf("unexpected error closing temp file: %v", err)
	}

	pArgs, err := LoadArgs("--config", cfg.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedResourceExclude := resourcemonitor.ResourceExclude{
		"masternode":  {"memory", "device/exampleA"},
		"workernode1": {"memory", "device/exampleB"},
		"workernode2": {"cpu"},
		"*":           {"device/exampleC"},
	}

	if !reflect.DeepEqual(pArgs.Resourcemonitor.ResourceExclude, expectedResourceExclude) {
		t.Errorf("ResourceExclude is different!\ngot=%+#v\nexpected=%+#v", pArgs.Resourcemonitor.ResourceExclude, expectedResourceExclude)
	}
}

func setupTest(t *testing.T) func() {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("Cannot retrieve tests directory")
	}

	baseDir = filepath.Dir(file)
	testDataDir = filepath.Clean(filepath.Join(baseDir, "..", "..", "test", "data"))

	return envSetter(map[string]string{
		"NODE_NAME":                node,
		"REFERENCE_NAMESPACE":      namespace,
		"REFERENCE_POD_NAME":       pod,
		"REFERENCE_CONTAINER_NAME": container,
	})
}

func envSetter(envs map[string]string) (closer func()) {
	originalEnvs := map[string]string{}

	for name, value := range envs {
		if originalValue, ok := os.LookupEnv(name); ok {
			originalEnvs[name] = originalValue
		}
		_ = os.Setenv(name, value)
	}

	return func() {
		for name := range envs {
			origValue, has := originalEnvs[name]
			if has {
				_ = os.Setenv(name, origValue)
			} else {
				_ = os.Unsetenv(name)
			}
		}
	}
}
