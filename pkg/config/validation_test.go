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
