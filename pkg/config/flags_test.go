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
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres/middleware/sharedcpuspool"
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
	_, closer := setupTest(t)
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
	_, closer := setupTest(t)
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
	_, closer := setupTest(t)
	t.Cleanup(closer)

	pArgs, err := LoadArgs("--refresh-node-resources")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !pArgs.Resourcemonitor.RefreshNodeResources {
		t.Errorf("refresh node resources not enabled")
	}
}

func TestLoadDefaults(t *testing.T) {
	_, closer := setupTest(t)
	t.Cleanup(closer)

	pArgs, err := LoadArgs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pArgsAsJson := pArgs.ToJSONString()

	golden := filepath.Join(testDataDir, fmt.Sprintf("%s.expected.json", t.Name()))
	if *update {
		err = os.WriteFile(golden, []byte(pArgsAsJson), 0644)

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

func setupTest(t *testing.T) (string, func()) {
	t.Helper()
	return setupTestWithEnv(t, map[string]string{
		"NODE_NAME":                node,
		"REFERENCE_NAMESPACE":      namespace,
		"REFERENCE_POD_NAME":       pod,
		"REFERENCE_CONTAINER_NAME": container,
	})
}

func setupTestWithEnv(t *testing.T, envs map[string]string) (string, func()) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("Cannot retrieve tests directory")
	}

	baseDir = filepath.Dir(file)
	testDataDir = filepath.Clean(filepath.Join(baseDir, "..", "..", "test", "data"))

	userDir, err := UserRunDir()
	if err == nil {
		err2 := os.CopyFS(userDir, os.DirFS(testDataDir))
		log.Printf("copyfs %q -> %q err=%v", testDataDir, userDir, err2)
	}

	closer := envSetter(envs)
	return userDir, func() {
		closer()
		if err != nil {
			return
		}
		err2 := os.RemoveAll(userDir)
		log.Printf("cleaned up %q err=%v", userDir, err2)
	}
}

func envSetter(envs map[string]string) func() {
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
