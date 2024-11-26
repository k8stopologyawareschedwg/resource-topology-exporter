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
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcemonitor"
)

func TestReadResourceExclude(t *testing.T) {
	testDir, closer := setupTest(t)
	t.Cleanup(closer)

	cfg, err := os.CreateTemp(testDir, "exclude-list")
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
			extraPath := FixExtraConfigPath(confRoot)

			var pArgs ProgArgs
			SetDefaults(&pArgs)
			err := FromFiles(&pArgs, confRoot, extraPath)
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
