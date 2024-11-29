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
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcemonitor"
)

func TestReadResourceExclude(t *testing.T) {
	testDir, closer := setupTest(t)
	t.Cleanup(closer)

	err := os.MkdirAll(filepath.Join(testDir, "extra"), 0755)
	if err != nil {
		t.Fatalf("unexpected error creating temp file: %v", err)
	}

	cfgContent := `resourceExclude:
  masternode: [memory, device/exampleA]
  workernode1: [memory, device/exampleB]
  workernode2: [cpu]
  "*": [device/exampleC]`

	if err := os.WriteFile(filepath.Join(testDir, "extra", "config.yaml"), []byte(cfgContent), 0644); err != nil {
		t.Fatalf("unexpected error writing data: %v", err)
	}

	pArgs, err := LoadArgs(testDir)
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

func TestFromFiles(t *testing.T) {
	testDir, closer := setupTest(t)
	t.Cleanup(closer)

	type testCase struct {
		name string
	}

	for _, tcase := range []testCase{
		{
			name: "00-full",
		},
	} {
		t.Run(tcase.name, func(t *testing.T) {
			confRoot := filepath.Join(testDir, "conftree", tcase.name)
			cleanCase := setupCase(t, testDir, tcase.name)
			t.Cleanup(cleanCase)

			var pArgs ProgArgs
			SetDefaults(&pArgs)
			err := FromFiles(&pArgs, testDir)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			golden := filepath.Join(confRoot, "_output", "output.yaml")

			expectedRaw, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got := strings.TrimSpace(string(pArgs.ToYAMLString()))
			expected := strings.TrimSpace(string(expectedRaw))
			if got != expected {
				t.Errorf("invalid defaults.\n>>> got:\n{%s}\n>>> expected:\n{%s}", got, expected)
			}
		})
	}
}

func setupCase(t *testing.T, testDir, name string) func() {
	t.Helper()

	log.Printf("setupCase: %s", name)

	confRoot := filepath.Join(testDir, "conftree", name)

	if err := os.CopyFS(filepath.Join(testDir, "daemon"), os.DirFS(filepath.Join(confRoot, "daemon"))); err != nil {
		t.Fatalf("cannot setup daemon: %v", err)
	}
	log.Printf("copied: %s -> %s", filepath.Join(confRoot, "daemon"), filepath.Join(testDir, "daemon"))

	if err := os.CopyFS(filepath.Join(testDir, "extra"), os.DirFS(filepath.Join(confRoot, "extra"))); err != nil {
		t.Fatalf("cannot setup extra: %v", err)
	}
	log.Printf("copied: %s -> %s", filepath.Join(confRoot, "extra"), filepath.Join(testDir, "extra"))

	return func() {
		var err error
		err = os.RemoveAll(filepath.Join(testDir, "daemon"))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("cannot cleanup daemon: %v", err)
		}
		log.Printf("cleanup: %s", filepath.Join(testDir, "daemon"))
		err = os.RemoveAll(filepath.Join(testDir, "extra"))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("cannot cleanup extra: %v", err)
		}
		log.Printf("cleanup: %s", filepath.Join(testDir, "extra"))
	}
}
