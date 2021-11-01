/*
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

package sysinfo

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestHugepageResourceName(t *testing.T) {
	type testCase struct {
		sizeKB   int
		expected string
	}

	testCases := []testCase{
		{
			sizeKB:   2 * 1024,
			expected: "hugepages-2Mi",
		},
		{
			sizeKB:   1 * 1024 * 1024,
			expected: "hugepages-1Gi",
		},
		// weird cases unexpected on x86_64
		{
			sizeKB:   8,
			expected: "hugepages-8Ki",
		},
		{
			sizeKB:   4 * 1024,
			expected: "hugepages-4Mi",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			got := HugepageResourceNameFromSize(tc.sizeKB)
			if tc.expected != got {
				t.Fatalf("hugepage sizeKB=%d expected=%q got=%q", tc.sizeKB, tc.expected, got)
			}
		})
	}

}

func TestHugepagesForNode(t *testing.T) {
	rootDir, err := os.MkdirTemp("", "fakehp")
	if err != nil {
		t.Errorf("failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(rootDir) // clean up

	if err := makeTree(rootDir, 2); err != nil {
		t.Errorf("failed to setup the fake tree on %q: %v", rootDir, err)
	}
	if err := setHPCount(rootDir, 0, HugepageSize2Mi, 6); err != nil {
		t.Errorf("failed to setup hugepages on node %d the fake tree on %q: %v", 0, rootDir, err)
	}
	if err := setHPCount(rootDir, 1, HugepageSize2Mi, 8); err != nil {
		t.Errorf("failed to setup hugepages on node %d the fake tree on %q: %v", 0, rootDir, err)
	}

	hpCounters, err := GetMemoryResourceCounters(Handle{rootDir})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(hpCounters["hugepages-2Mi"]) != 2 {
		t.Errorf("found unexpected 2Mi hugepages: %v", hpCounters["hugepages-2Mi"])
	}
	if hpCounters["hugepages-2Mi"][0] != 12582912 {
		t.Errorf("found unexpected 2Mi hugepages for node 0: %v", hpCounters["hugepages-2Mi"][0])
	}
	if hpCounters["hugepages-2Mi"][1] != 16777216 {
		t.Errorf("found unexpected 2Mi hugepages for node 1: %v", hpCounters["hugepages-2Mi"][1])
	}

	if len(hpCounters["hugepages-1Gi"]) != 2 {
		t.Errorf("found unexpected 1Gi hugepages")
	}
	if hpCounters["hugepages-1Gi"][0] != 0 {
		t.Errorf("found unexpected 1Gi hugepages for node 0: %v", hpCounters["hugepages-1Gi"][0])
	}
	if hpCounters["hugepages-1Gi"][1] != 0 {
		t.Errorf("found unexpected 1Gi hugepages for node 1: %v", hpCounters["hugepages-1Gi"][1])
	}
}

func makeTree(root string, numNodes int) error {
	hnd := Handle{root}
	for idx := 0; idx < numNodes; idx++ {
		for _, size := range []int{HugepageSize2Mi, HugepageSize1Gi} {
			path := filepath.Join(
				hnd.SysDevicesNodesNodeNth(idx),
				"hugepages",
				fmt.Sprintf("hugepages-%dkB", size),
			)
			if err := os.MkdirAll(path, 0755); err != nil {
				return err
			}
			if err := setHPCount(root, idx, size, 0); err != nil {
				return err
			}
		}
	}
	return nil
}

func setHPCount(root string, nodeID, pageSize, numPages int) error {
	hnd := Handle{root}
	path := filepath.Join(
		hnd.SysDevicesNodesNodeNth(nodeID),
		"hugepages",
		fmt.Sprintf("hugepages-%dkB", pageSize),
		"nr_hugepages",
	)
	return os.WriteFile(path, []byte(fmt.Sprintf("%d", numPages)), 0644)
}
