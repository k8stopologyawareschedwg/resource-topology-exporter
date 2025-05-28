package filter

import (
	"testing"

	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"
)

func TestAlwaysPass(t *testing.T) {
	type testCase struct {
		name string
		pr   *podresourcesapi.PodResources
	}

	testCases := []testCase{
		{
			name: "nil reference",
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
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := VerifyAlwaysPass(tc.pr)
			if !got.Allow {
				t.Fatalf("alwayspass failed")
			}
		})
	}
}
