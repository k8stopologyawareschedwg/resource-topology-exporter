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

package topologypolicy

import (
	"fmt"
	"testing"

	v1alpha2 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha2"
)

func TestHugepageResourceName(t *testing.T) {
	type testCase struct {
		policy   string
		scope    string
		expected v1alpha2.TopologyManagerPolicy
	}

	testCases := []testCase{
		{
			policy:   "single-numa-node",
			scope:    "container",
			expected: v1alpha2.SingleNUMANodeContainerLevel,
		},
		{
			policy:   "single-numa-node",
			scope:    "pod",
			expected: v1alpha2.SingleNUMANodePodLevel,
		},
		{
			policy:   "restricted",
			scope:    "container",
			expected: v1alpha2.Restricted,
		},
		{
			policy:   "restricted",
			scope:    "pod",
			expected: v1alpha2.Restricted,
		},
		{
			policy:   "best-effort",
			scope:    "container",
			expected: v1alpha2.BestEffort,
		},
		{
			policy:   "best-effort",
			scope:    "pod",
			expected: v1alpha2.BestEffort,
		},
		{
			policy:   "none",
			scope:    "container",
			expected: v1alpha2.None,
		},
		{
			policy:   "none",
			scope:    "pod",
			expected: v1alpha2.None,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("policy=%q scope=%q", tc.policy, tc.scope), func(t *testing.T) {
			got := DetectTopologyPolicy(tc.policy, tc.scope)
			if tc.expected != got {
				t.Fatalf("expected=%q got=%q", tc.expected, got)
			}
		})
	}

}
