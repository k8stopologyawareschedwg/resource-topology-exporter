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

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestLoadArgs(t *testing.T) {
	type testCase struct {
		name string
	}

	for _, tcase := range []testCase{
		{
			name: "05-full-env",
		},
		{
			name: "06-full-env-args",
		},
	} {
		t.Run(tcase.name, func(t *testing.T) {
			confRoot := filepath.Join(testDataDir, "conftree", tcase.name)

			environ := filepath.Join(confRoot, "_env", "vars.yaml")
			setupEnviron(t, environ)

			cmdline := filepath.Join(confRoot, "_cmdline", "flags.yaml")
			args := setupCmdline(t, cmdline)
			args = append(args, confRoot) // always last

			pArgs, err := LoadArgs(args...)
			if err != nil {
				t.Fatalf("LoadArgs(%s) failed: %v", confRoot, err)
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

func setupEnviron(t *testing.T, envDefsPath string) {
	t.Helper()
	data, err := os.ReadFile(envDefsPath)
	if err != nil {
		// intentionally swallow
		return
	}
	var envVars map[string]string
	err = yaml.Unmarshal(data, &envVars)
	if err != nil {
		t.Logf("error getting environment definitions from %q: %v", envDefsPath, err)
		// intentionally swallow
		return
	}
	for key, val := range envVars {
		t.Logf("setup environ: %q -> %q", key, val)
		t.Setenv(key, val)
	}
}

func setupCmdline(t *testing.T, cmdlineDefsPath string) []string {
	t.Helper()
	var flags []string
	data, err := os.ReadFile(cmdlineDefsPath)
	if err != nil {
		// intentionally swallow
		return flags
	}
	err = yaml.Unmarshal(data, &flags)
	if err != nil {
		t.Logf("error getting commandline flags from %q: %v", cmdlineDefsPath, err)
		// intentionally swallow
		return flags
	}
	t.Logf("using command line: %q", flags)
	return flags
}
