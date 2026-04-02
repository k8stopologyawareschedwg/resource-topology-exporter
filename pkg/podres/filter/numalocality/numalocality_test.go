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

func TestIsPresent(t *testing.T) {
	type testCase struct {
		name     string
		topo     *podresourcesapi.TopologyInfo
		expected bool
	}

	testCases := []testCase{
		{
			name:     "nil",
			topo:     nil,
			expected: false,
		},
		{
			name:     "nil nodes",
			topo:     &podresourcesapi.TopologyInfo{},
			expected: false,
		},
		{
			name: "empty nodes",
			topo: &podresourcesapi.TopologyInfo{
				Nodes: []*podresourcesapi.NUMANode{},
			},
			expected: false,
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
			expected: false,
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
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsPresent(tc.topo)
			if tc.expected != got {
				t.Fatalf("expected=%v got=%v", tc.expected, got)
			}
		})
	}
}

func TestFindContainerSingleNUMAPlacement(t *testing.T) {
	coreIDToNodeIDMap := map[int]int{
		0: 0, 1: 0, 2: 0, 3: 0,
		4: 1, 5: 1, 6: 1, 7: 1,
	}

	type testCase struct {
		name        string
		cnt         *podresourcesapi.ContainerResources
		expectedID  int
		expectError bool
	}

	testCases := []testCase{
		{
			name:        "nil container",
			cnt:         nil,
			expectedID:  -1,
			expectError: true,
		},
		{
			name:       "no resources at all",
			cnt:        &podresourcesapi.ContainerResources{Name: "empty"},
			expectedID: -1,
		},
		{
			name: "exclusive CPUs on NUMA 0",
			cnt: &podresourcesapi.ContainerResources{
				Name:   "cpu-numa0",
				CpuIds: []int64{0, 2},
			},
			expectedID: 0,
		},
		{
			name: "exclusive CPUs on NUMA 1",
			cnt: &podresourcesapi.ContainerResources{
				Name:   "cpu-numa1",
				CpuIds: []int64{4, 5, 6},
			},
			expectedID: 1,
		},
		{
			name: "CPU ID not in map",
			cnt: &podresourcesapi.ContainerResources{
				Name:   "cpu-unknown",
				CpuIds: []int64{99},
			},
			expectedID:  -1,
			expectError: true,
		},
		{
			name: "device with topology on NUMA 1",
			cnt: &podresourcesapi.ContainerResources{
				Name: "dev-numa1",
				Devices: []*podresourcesapi.ContainerDevices{
					{
						ResourceName: "example.com/gpu",
						DeviceIds:    []string{"gpu-0"},
						Topology: &podresourcesapi.TopologyInfo{
							Nodes: []*podresourcesapi.NUMANode{{ID: 1}},
						},
					},
				},
			},
			expectedID: 1,
		},
		{
			name: "device without topology is skipped",
			cnt: &podresourcesapi.ContainerResources{
				Name: "dev-no-topo",
				Devices: []*podresourcesapi.ContainerDevices{
					{
						ResourceName: "example.com/nic",
						DeviceIds:    []string{"nic-0"},
					},
				},
			},
			expectedID: -1,
		},
		{
			name: "device with nil topology nodes is skipped",
			cnt: &podresourcesapi.ContainerResources{
				Name: "dev-nil-nodes",
				Devices: []*podresourcesapi.ContainerDevices{
					{
						ResourceName: "example.com/nic",
						DeviceIds:    []string{"nic-0"},
						Topology: &podresourcesapi.TopologyInfo{
							Nodes: []*podresourcesapi.NUMANode{{ID: -1}},
						},
					},
				},
			},
			expectedID: -1,
		},
		{
			name: "device with no device IDs is skipped",
			cnt: &podresourcesapi.ContainerResources{
				Name: "dev-no-ids",
				Devices: []*podresourcesapi.ContainerDevices{
					{
						ResourceName: "example.com/gpu",
						Topology: &podresourcesapi.TopologyInfo{
							Nodes: []*podresourcesapi.NUMANode{{ID: 0}},
						},
					},
				},
			},
			expectedID: -1,
		},
		{
			name: "memory with topology on NUMA 0",
			cnt: &podresourcesapi.ContainerResources{
				Name: "mem-numa0",
				Memory: []*podresourcesapi.ContainerMemory{
					{
						MemoryType: "memory",
						Size:       1073741824,
						Topology: &podresourcesapi.TopologyInfo{
							Nodes: []*podresourcesapi.NUMANode{{ID: 0}},
						},
					},
				},
			},
			expectedID: 0,
		},
		{
			name: "memory without topology is skipped",
			cnt: &podresourcesapi.ContainerResources{
				Name: "mem-no-topo",
				Memory: []*podresourcesapi.ContainerMemory{
					{
						MemoryType: "memory",
						Size:       1073741824,
					},
				},
			},
			expectedID: -1,
		},
		{
			name: "CPUs take priority over devices",
			cnt: &podresourcesapi.ContainerResources{
				Name:   "cpu-and-dev",
				CpuIds: []int64{0},
				Devices: []*podresourcesapi.ContainerDevices{
					{
						ResourceName: "example.com/gpu",
						DeviceIds:    []string{"gpu-0"},
						Topology: &podresourcesapi.TopologyInfo{
							Nodes: []*podresourcesapi.NUMANode{{ID: 1}},
						},
					},
				},
			},
			expectedID: 0,
		},
		{
			name: "first valid device with topology wins",
			cnt: &podresourcesapi.ContainerResources{
				Name: "multi-dev",
				Devices: []*podresourcesapi.ContainerDevices{
					{
						ResourceName: "example.com/nic",
						DeviceIds:    []string{"nic-0"},
					},
					{
						ResourceName: "example.com/gpu",
						DeviceIds:    []string{"gpu-0"},
						Topology: &podresourcesapi.TopologyInfo{
							Nodes: []*podresourcesapi.NUMANode{{ID: 1}},
						},
					},
				},
			},
			expectedID: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FindContainerSingleNUMAPlacement(coreIDToNodeIDMap, tc.cnt)
			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expectedID {
				t.Fatalf("expected NUMA node %d, got %d", tc.expectedID, got)
			}
		})
	}
}
