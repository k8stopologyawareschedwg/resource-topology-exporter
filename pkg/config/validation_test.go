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
	"errors"
	"testing"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcemonitor"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourcetopologyexporter"
)

func TestValidation(t *testing.T) {
	type testCase struct {
		name          string
		pArgs         ProgArgs
		expectedError bool
	}

	for _, tcase := range []testCase{
		{
			name:          "all empty",
			pArgs:         ProgArgs{},
			expectedError: true,
		},
		{
			name: "all correct",
			pArgs: ProgArgs{
				RTE: resourcetopologyexporter.Args{
					MetricsMode: "http",
				},
				Resourcemonitor: resourcemonitor.Args{
					PodSetFingerprintMethod: "all",
				},
			},
			expectedError: false,
		},
		{
			name: "invalid metrics mode",
			pArgs: ProgArgs{
				RTE: resourcetopologyexporter.Args{
					MetricsMode: "foo",
				},
				Resourcemonitor: resourcemonitor.Args{
					PodSetFingerprintMethod: "all",
				},
			},
			expectedError: true,
		},
		{
			name: "invalid pfp method",
			pArgs: ProgArgs{
				RTE: resourcetopologyexporter.Args{
					MetricsMode: "http",
				},
				Resourcemonitor: resourcemonitor.Args{
					PodSetFingerprintMethod: "foobar",
				},
			},
			expectedError: true,
		},
		{
			name: "both invalud",
			pArgs: ProgArgs{
				RTE: resourcetopologyexporter.Args{
					MetricsMode: "_foobar_",
				},
				Resourcemonitor: resourcemonitor.Args{
					PodSetFingerprintMethod: "__bad",
				},
			},
			expectedError: true,
		},
	} {
		t.Run(tcase.name, func(t *testing.T) {
			err := Validate(&tcase.pArgs)
			gotErr := (err != nil)
			if gotErr != tcase.expectedError {
				t.Errorf("validation mismatch: got %v expected %v", gotErr, tcase.expectedError)
			}
		})
	}
}

func TestIsConfigRootAllowed(t *testing.T) {
	type testCase struct {
		name            string
		cfgPath         string
		addDirFns       []func() (string, error)
		expectedError   bool
		expectedPattern string
	}

	// important note: this functions *expects* canonicalized paths
	for _, tcase := range []testCase{
		{
			name:            "base path",
			cfgPath:         "/etc/rte",
			addDirFns:       []func() (string, error){},
			expectedPattern: "/etc/rte",
		},
		{
			name:            "obvious misuses",
			cfgPath:         "/etc/passwd",
			addDirFns:       []func() (string, error){},
			expectedPattern: "",
		},
		{
			name:            "slightly less obvious misuse",
			cfgPath:         "/usr/local/etc/shadow",
			addDirFns:       []func() (string, error){},
			expectedPattern: "",
		},
		{
			name:    "injection fails on err",
			cfgPath: "/var/rte/config",
			addDirFns: []func() (string, error){
				func() (string, error) {
					return "/var/rte/config", errors.New("bogus error")
				},
			},
			expectedPattern: "",
		},
		{
			name:    "injected run",
			cfgPath: "/run/user/12345/rte",
			addDirFns: []func() (string, error){
				func() (string, error) {
					return "/run/user/12345/rte", nil
				},
			},
			expectedPattern: "/run/user/12345/rte",
		},
		{
			name:            "run not allowed unless injected",
			cfgPath:         "/run/user/12345/rte",
			addDirFns:       []func() (string, error){},
			expectedPattern: "",
		},
		{
			name:    "injected home",
			cfgPath: "/home/foobar/.rte",
			addDirFns: []func() (string, error){
				func() (string, error) {
					return "/home/foobar/.rte", nil
				},
			},
			expectedPattern: "/home/foobar/.rte",
		},
		{
			name:            "home not allowed unless injected",
			cfgPath:         "/home/foobar/.rte",
			addDirFns:       []func() (string, error){},
			expectedPattern: "",
		},
	} {
		t.Run(tcase.name, func(t *testing.T) {
			pattern, _ := IsConfigRootAllowed(tcase.cfgPath, tcase.addDirFns...)
			if pattern != tcase.expectedPattern {
				t.Errorf("validation mismatch: got pattern %q expected %q", pattern, tcase.expectedPattern)
			}
		})
	}
}
