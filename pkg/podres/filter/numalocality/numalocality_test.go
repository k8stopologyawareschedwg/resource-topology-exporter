package numalocality

import (
	"testing"

	podresourcesapi "github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres/api/v1"
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

// {"pod_resources":[{"name":"image-registry-78b84dc9f9-zwxtk","namespace":"image-registry","containers":[{"name":"registry"}]}]}
// {"pod_resources":[{"name":"image-registry-78b84dc9f9-zwxtk","namespace":"image-registry","containers":[{"name":"registry"}]}, {"name":"network-check-source-677bdb7d9-lqrcb","namespace":"network-diagnostics","containers":[{"name":"check-endpoints"}]},{"name":"network-check-target-m9mlq","namespace":"network-diagnostics","containers":[{"name":"network-check-target-container"}]}]}
