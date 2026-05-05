package numalocality

import (
	"testing"

	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"
)

func TestVerify(t *testing.T) {
	type testCase struct {
		name     string
		pr       *podresourcesapi.PodResources
		expected bool
	}

	testCases := []testCase{
		{
			name:     "nil reference",
			expected: false,
		},
		{
			name: "no exclusive resources",
			pr: &podresourcesapi.PodResources{
				Name:      "image-registry-78b84dc9f9-zwxtk",
				Namespace: "image-registry",
				Containers: []*podresourcesapi.ContainerResources{
					{
						Name: "registry",
					},
				},
			},
			expected: false,
		},
		{
			name: "exclusive CPUs",
			pr: &podresourcesapi.PodResources{
				Name:      "highperf-cpus",
				Namespace: "exclusive-resources",
				Containers: []*podresourcesapi.ContainerResources{
					{
						Name:   "compute-intensive",
						CpuIds: []int64{0, 2, 4, 6},
					},
				},
			},
			expected: true,
		},
		{
			name: "have devices no topology",
			pr: &podresourcesapi.PodResources{
				Name:      "highperf-devs-no-topology",
				Namespace: "exclusive-resources",
				Containers: []*podresourcesapi.ContainerResources{
					{
						Name: "require-devices",
						Devices: []*podresourcesapi.ContainerDevices{
							{
								ResourceName: "fancydev",
								DeviceIds:    []string{"dev-1", "dev-2"},
							},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "have devices with topology",
			pr: &podresourcesapi.PodResources{
				Name:      "highperf-devs-with-topology",
				Namespace: "exclusive-resources",
				Containers: []*podresourcesapi.ContainerResources{
					{
						Name: "require-devices",
						Devices: []*podresourcesapi.ContainerDevices{
							{
								ResourceName: "fancydev",
								DeviceIds:    []string{"dev-1", "dev-2"},
							},
						},
					},
				},
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := Verify(tc.pr)
			if tc.expected != got.Allow {
				t.Fatalf("expected=%v got=%v", tc.expected, got)
			}
		})
	}
}

func TestGetNUMAID(t *testing.T) {
	type testCase struct {
		name     string
		topo     *podresourcesapi.TopologyInfo
		expected int
	}

	testCases := []testCase{
		{
			name:     "nil",
			topo:     nil,
			expected: -1,
		},
		{
			name:     "nil nodes",
			topo:     &podresourcesapi.TopologyInfo{},
			expected: -1,
		},
		{
			name: "empty nodes",
			topo: &podresourcesapi.TopologyInfo{
				Nodes: []*podresourcesapi.NUMANode{},
			},
			expected: -1,
		},
		{
			name: "any NUMA locality",
			topo: &podresourcesapi.TopologyInfo{
				Nodes: []*podresourcesapi.NUMANode{
					{
						ID: -1,
					},
				},
			},
			expected: -1,
		},
		{
			name: "defined NUMA locality",
			topo: &podresourcesapi.TopologyInfo{
				Nodes: []*podresourcesapi.NUMANode{
					{
						ID: 1,
					},
				},
			},
			expected: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := GetNUMAID(tc.topo)
			if tc.expected != got {
				t.Fatalf("expected=%v got=%v", tc.expected, got)
			}
		})
	}
}

func TestResolveContainerPlacement(t *testing.T) {
	coreIDToNodeIDMap := map[int]int{
		0: 0,
		1: 1,
	}

	type testCase struct {
		name             string
		cnt              *podresourcesapi.ContainerResources
		expectedEligible bool
		expectedNodeID   int
		expectError      bool
	}

	testCases := []testCase{
		{
			name:             "nil container",
			cnt:              nil,
			expectedEligible: false,
			expectedNodeID:   -1,
			expectError:      true,
		},
		{
			name: "cpu placement found",
			cnt: &podresourcesapi.ContainerResources{
				Name:   "cpu-found",
				CpuIds: []int64{1},
			},
			expectedEligible: true,
			expectedNodeID:   1,
			expectError:      false,
		},
		{
			name: "cpu placement missing from map",
			cnt: &podresourcesapi.ContainerResources{
				Name:   "cpu-missing",
				CpuIds: []int64{9},
			},
			// Missing CPU in core map is treated as an error while still eligible for placement semantics.
			expectedEligible: true,
			expectedNodeID:   -1,
			expectError:      true,
		},
		{
			name: "device placement found",
			cnt: &podresourcesapi.ContainerResources{
				Name: "dev-found",
				Devices: []*podresourcesapi.ContainerDevices{
					{
						ResourceName: "example.com/gpu",
						DeviceIds:    []string{"gpu-0"},
						Topology: &podresourcesapi.TopologyInfo{
							Nodes: []*podresourcesapi.NUMANode{{ID: 0}},
						},
					},
				},
			},
			expectedEligible: true,
			expectedNodeID:   0,
			expectError:      false,
		},
		{
			name: "memory placement found after non-matching device",
			cnt: &podresourcesapi.ContainerResources{
				Name: "mem-found",
				Devices: []*podresourcesapi.ContainerDevices{
					{
						ResourceName: "example.com/fpga",
						DeviceIds:    []string{"fpga-0"},
						Topology: &podresourcesapi.TopologyInfo{
							Nodes: []*podresourcesapi.NUMANode{{ID: -1}},
						},
					},
				},
				Memory: []*podresourcesapi.ContainerMemory{
					{
						MemoryType: "memory",
						Size:       1024,
						Topology: &podresourcesapi.TopologyInfo{
							Nodes: []*podresourcesapi.NUMANode{{ID: 1}},
						},
					},
				},
			},
			expectedEligible: true,
			expectedNodeID:   1,
			expectError:      false,
		},
		{
			name: "no placement detected",
			cnt: &podresourcesapi.ContainerResources{
				Name: "no-placement",
				Devices: []*podresourcesapi.ContainerDevices{
					{
						ResourceName: "example.com/nic",
						DeviceIds:    []string{},
						Topology: &podresourcesapi.TopologyInfo{
							Nodes: []*podresourcesapi.NUMANode{{ID: 0}},
						},
					},
				},
				Memory: []*podresourcesapi.ContainerMemory{
					{
						MemoryType: "memory",
						Size:       1024,
						Topology: &podresourcesapi.TopologyInfo{
							Nodes: []*podresourcesapi.NUMANode{{ID: -1}},
						},
					},
				},
			},
			expectedEligible: false,
			expectedNodeID:   -1,
			expectError:      false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			eligible, nodeID, err := ResolveContainerPlacement(coreIDToNodeIDMap, tc.cnt)
			if tc.expectError && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.expectError && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if eligible != tc.expectedEligible {
				t.Fatalf("expected eligible=%v got=%v", tc.expectedEligible, eligible)
			}
			if nodeID != tc.expectedNodeID {
				t.Fatalf("expected nodeID=%d got=%d", tc.expectedNodeID, nodeID)
			}
		})
	}
}
