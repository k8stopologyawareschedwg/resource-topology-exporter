/*
Copyright 2020 The Kubernetes Authors.

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

package resourcemonitor

import (
	"encoding/json"
	"log"
	"sort"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"
	v1 "k8s.io/kubelet/pkg/apis/podresources/v1"

	cmp "github.com/google/go-cmp/cmp"
	"github.com/jaypipes/ghw"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	topologyv1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/sysinfo"
)

func TestMakeCoreIDToNodeIDMap(t *testing.T) {
	fakeTopo := ghw.TopologyInfo{}
	Convey("When recovering test topology from JSON data", t, func() {
		err := json.Unmarshal([]byte(testTopology), &fakeTopo)
		So(err, ShouldBeNil)
	})

	Convey("When mapping cores to nodes", t, func() {
		res := MakeCoreIDToNodeIDMap(&fakeTopo)
		expected := getExpectedCoreToNodeMap()
		log.Printf("result=%v", res)
		log.Printf("expected=%v", expected)
		log.Printf("diff=%s", cmp.Diff(res, expected))
		So(cmp.Equal(res, expected), ShouldBeTrue)
	})

}

func TestNormalizeContainerDevices(t *testing.T) {
	availRes := &v1.AllocatableResourcesResponse{
		Devices: []*v1.ContainerDevices{
			{
				ResourceName: "fake.io/net",
				DeviceIds:    []string{"netAAA-0"},
				Topology: &v1.TopologyInfo{
					Nodes: []*v1.NUMANode{
						{
							ID: 0,
						},
					},
				},
			},
			{
				ResourceName: "fake.io/net",
				DeviceIds:    []string{"netAAA-1"},
				Topology: &v1.TopologyInfo{
					Nodes: []*v1.NUMANode{
						{
							ID: 0,
						},
					},
				},
			},
			{
				ResourceName: "fake.io/net",
				DeviceIds:    []string{"netAAA-2"},
				Topology: &v1.TopologyInfo{
					Nodes: []*v1.NUMANode{
						{
							ID: 0,
						},
					},
				},
			},
			{
				ResourceName: "fake.io/net",
				DeviceIds:    []string{"netAAA-3"},
				Topology: &v1.TopologyInfo{
					Nodes: []*v1.NUMANode{
						{
							ID: 0,
						},
					},
				},
			},
			{
				ResourceName: "fake.io/net",
				DeviceIds:    []string{"netBBB-0"},
				Topology: &v1.TopologyInfo{
					Nodes: []*v1.NUMANode{
						{
							ID: 1,
						},
					},
				},
			},
			{
				ResourceName: "fake.io/net",
				DeviceIds:    []string{"netBBB-1"},
				Topology: &v1.TopologyInfo{
					Nodes: []*v1.NUMANode{
						{
							ID: 1,
						},
					},
				},
			},
			{
				ResourceName: "fake.io/gpu",
				DeviceIds:    []string{"gpuAAA"},
				Topology: &v1.TopologyInfo{
					Nodes: []*v1.NUMANode{
						{
							ID: 1,
						},
					},
				},
			},
		},
		Memory: []*v1.ContainerMemory{
			{
				MemoryType: "memory",
				Size_:      1024,
				Topology: &v1.TopologyInfo{
					Nodes: []*v1.NUMANode{
						{
							ID: 0,
						},
					},
				},
			},
			{
				MemoryType: "memory",
				Size_:      1024,
				Topology: &v1.TopologyInfo{
					Nodes: []*v1.NUMANode{
						{
							ID: 1,
						},
					},
				},
			},
			{
				MemoryType: "hugepages-2Mi",
				Size_:      1024,
				Topology: &v1.TopologyInfo{
					Nodes: []*v1.NUMANode{
						{
							ID: 1,
						},
					},
				},
			},
		},
		CpuIds: []int64{
			0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11,
			12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23,
		},
	}

	coreIDToNodeIDMap := getExpectedCoreToNodeMap()

	Convey("When normalizing the container devices from pod resources", t, func() {
		topo := ghw.TopologyInfo{
			Nodes: []*ghw.TopologyNode{
				{ID: 0},
				{ID: 1},
			},
		}
		res := NormalizeContainerDevices(availRes.GetDevices(), availRes.GetMemory(), availRes.GetCpuIds(), &topo, coreIDToNodeIDMap)
		expected := []*podresourcesapi.ContainerDevices{
			{
				ResourceName: "fake.io/net",
				DeviceIds:    []string{"netAAA-0"},
				Topology: &podresourcesapi.TopologyInfo{
					Nodes: []*podresourcesapi.NUMANode{
						{
							ID: 0,
						},
					},
				},
			},
			{
				ResourceName: "fake.io/net",
				DeviceIds:    []string{"netAAA-1"},
				Topology: &podresourcesapi.TopologyInfo{
					Nodes: []*podresourcesapi.NUMANode{
						{
							ID: 0,
						},
					},
				},
			},
			{
				ResourceName: "fake.io/net",
				DeviceIds:    []string{"netAAA-2"},
				Topology: &podresourcesapi.TopologyInfo{
					Nodes: []*podresourcesapi.NUMANode{
						{
							ID: 0,
						},
					},
				},
			},
			{
				ResourceName: "fake.io/net",
				DeviceIds:    []string{"netAAA-3"},
				Topology: &podresourcesapi.TopologyInfo{
					Nodes: []*podresourcesapi.NUMANode{
						{
							ID: 0,
						},
					},
				},
			},
			{
				ResourceName: "fake.io/net",
				DeviceIds:    []string{"netBBB-0"},
				Topology: &podresourcesapi.TopologyInfo{
					Nodes: []*podresourcesapi.NUMANode{
						{
							ID: 1,
						},
					},
				},
			},
			{
				ResourceName: "fake.io/net",
				DeviceIds:    []string{"netBBB-1"},
				Topology: &podresourcesapi.TopologyInfo{
					Nodes: []*podresourcesapi.NUMANode{
						{
							ID: 1,
						},
					},
				},
			},
			{
				ResourceName: "fake.io/gpu",
				DeviceIds:    []string{"gpuAAA"},
				Topology: &podresourcesapi.TopologyInfo{
					Nodes: []*podresourcesapi.NUMANode{
						{
							ID: 1,
						},
					},
				},
			},
			{
				ResourceName: "cpu",
				DeviceIds:    []string{"0", "2", "4", "6", "8", "10", "12", "14", "16", "18", "20", "22"},
				Topology: &podresourcesapi.TopologyInfo{
					Nodes: []*podresourcesapi.NUMANode{
						{
							ID: 0,
						},
					},
				},
			},
			{
				ResourceName: "cpu",
				DeviceIds:    []string{"1", "3", "5", "7", "9", "11", "13", "15", "17", "19", "21", "23"},
				Topology: &podresourcesapi.TopologyInfo{
					Nodes: []*podresourcesapi.NUMANode{
						{
							ID: 1,
						},
					},
				},
			},
			{
				ResourceName: "memory",
				DeviceIds:    []string{"1024"},
				Topology: &podresourcesapi.TopologyInfo{
					Nodes: []*podresourcesapi.NUMANode{
						{
							ID: 0,
						},
					},
				},
			},
			{
				ResourceName: "memory",
				DeviceIds:    []string{"1024"},
				Topology: &podresourcesapi.TopologyInfo{
					Nodes: []*podresourcesapi.NUMANode{
						{
							ID: 1,
						},
					},
				},
			},
			{
				ResourceName: "hugepages-2Mi",
				DeviceIds:    []string{"1024"},
				Topology: &podresourcesapi.TopologyInfo{
					Nodes: []*podresourcesapi.NUMANode{
						{
							ID: 1,
						},
					},
				},
			},
		}

		sort.Slice(res, func(i, j int) bool {
			if res[i].ResourceName == res[j].ResourceName {
				var sbi, sbj strings.Builder
				for _, id := range res[i].DeviceIds {
					sbi.WriteString(id)
				}

				for _, id := range res[j].DeviceIds {
					sbj.WriteString(id)
				}
				return sbi.String() < sbj.String()
			}
			return res[i].ResourceName < res[j].ResourceName
		})

		sort.Slice(expected, func(i, j int) bool {
			if expected[i].ResourceName == expected[j].ResourceName {
				var sbi, sbj strings.Builder
				for _, id := range expected[i].DeviceIds {
					sbi.WriteString(id)
				}

				for _, id := range expected[j].DeviceIds {
					sbj.WriteString(id)
				}
				return sbi.String() < sbj.String()
			}
			return expected[i].ResourceName < expected[j].ResourceName
		})

		log.Printf("result=%v", res)
		log.Printf("expected=%v", expected)
		log.Printf("diff=%s", cmp.Diff(res, expected))
		So(cmp.Equal(res, expected), ShouldBeTrue)
	})
}

// TODO: add testcase for
// - a pod with non-integral CPUs and devices, we need to not decrement the CPUs but do that for devices.

type fakeMemoryReporter struct {
	Memory    map[int]int64
	Hugepages []*sysinfo.Hugepages
}

func (fmr fakeMemoryReporter) GetMemory() (map[int]int64, error) {
	return fmr.Memory, nil
}

func (fmr fakeMemoryReporter) GetHugepages() ([]*sysinfo.Hugepages, error) {
	return fmr.Hugepages, nil
}

func TestResourcesScan(t *testing.T) {
	fakeTopo := ghw.TopologyInfo{
		Nodes: []*ghw.TopologyNode{
			{ID: 0},
			{ID: 1},
		},
	}
	fakeMemRep := fakeMemoryReporter{
		Memory: map[int]int64{
			0: int64(16638164992),
			1: int64(16638164992),
		},
	}

	Convey("When recovering test topology from JSON data", t, func() {
		err := json.Unmarshal([]byte(testTopology), &fakeTopo)
		So(err, ShouldBeNil)
	})

	allContainerDevices := []*v1.ContainerDevices{
		{
			ResourceName: "fake.io/net",
			DeviceIds:    []string{"netAAA-0"},
			Topology: &v1.TopologyInfo{
				Nodes: []*v1.NUMANode{
					{
						ID: 0,
					},
				},
			},
		},
		{
			ResourceName: "fake.io/net",
			DeviceIds:    []string{"netAAA-1"},
			Topology: &v1.TopologyInfo{
				Nodes: []*v1.NUMANode{
					{
						ID: 0,
					},
				},
			},
		},
		{
			ResourceName: "fake.io/net",
			DeviceIds:    []string{"netAAA-2"},
			Topology: &v1.TopologyInfo{
				Nodes: []*v1.NUMANode{
					{
						ID: 0,
					},
				},
			},
		},
		{
			ResourceName: "fake.io/net",
			DeviceIds:    []string{"netAAA-3"},
			Topology: &v1.TopologyInfo{
				Nodes: []*v1.NUMANode{
					{
						ID: 0,
					},
				},
			},
		},
		{
			ResourceName: "fake.io/net",
			DeviceIds:    []string{"netBBB-0"},
			Topology: &v1.TopologyInfo{
				Nodes: []*v1.NUMANode{
					{
						ID: 1,
					},
				},
			},
		},
		{
			ResourceName: "fake.io/net",
			DeviceIds:    []string{"netBBB-1"},
			Topology: &v1.TopologyInfo{
				Nodes: []*v1.NUMANode{
					{
						ID: 1,
					},
				},
			},
		},
		{
			ResourceName: "fake.io/gpu",
			DeviceIds:    []string{"gpuAAA"},
			Topology: &v1.TopologyInfo{
				Nodes: []*v1.NUMANode{
					{
						ID: 1,
					},
				},
			},
		},
	}

	Convey("When I aggregate the node resources fake data and no pod allocation", t, func() {
		availRes := &v1.AllocatableResourcesResponse{
			Devices: allContainerDevices,
			CpuIds: []int64{
				0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11,
				12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23,
			},
		}

		mockPodResClient := new(podres.MockPodResourcesListerClient)
		mockPodResClient.On("GetAllocatableResources", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("*v1.AllocatableResourcesRequest")).Return(availRes, nil)
		resMon, err := NewResourceMonitorWithParameters("TEST", &fakeTopo, fakeMemRep, mockPodResClient, Args{})
		So(err, ShouldBeNil)

		Convey("When aggregating resources", func() {
			expected := topologyv1alpha1.ZoneList{
				topologyv1alpha1.Zone{
					Name: "node-0",
					Type: "Node",
					Costs: topologyv1alpha1.CostList{
						topologyv1alpha1.CostInfo{
							Name:  "node-0",
							Value: 10,
						},
						topologyv1alpha1.CostInfo{
							Name:  "node-1",
							Value: 20,
						},
					},
					Resources: topologyv1alpha1.ResourceInfoList{
						topologyv1alpha1.ResourceInfo{
							Name:        "cpu",
							Available:   resource.MustParse("12"),
							Allocatable: resource.MustParse("12"),
							Capacity:    resource.MustParse("12"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "fake.io/net",
							Available:   resource.MustParse("4"),
							Allocatable: resource.MustParse("4"),
							Capacity:    resource.MustParse("4"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "memory",
							Available:   resource.MustParse("0"),
							Allocatable: resource.MustParse("0"),
							Capacity:    resource.MustParse("0"),
						},
					},
				},
				topologyv1alpha1.Zone{
					Name: "node-1",
					Type: "Node",
					Costs: topologyv1alpha1.CostList{
						topologyv1alpha1.CostInfo{
							Name:  "node-0",
							Value: 20,
						},
						topologyv1alpha1.CostInfo{
							Name:  "node-1",
							Value: 10,
						},
					},
					Resources: topologyv1alpha1.ResourceInfoList{
						topologyv1alpha1.ResourceInfo{
							Name:        "cpu",
							Available:   resource.MustParse("12"),
							Allocatable: resource.MustParse("12"),
							Capacity:    resource.MustParse("12"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "fake.io/gpu",
							Available:   resource.MustParse("1"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "fake.io/net",
							Available:   resource.MustParse("2"),
							Allocatable: resource.MustParse("2"),
							Capacity:    resource.MustParse("2"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "memory",
							Available:   resource.MustParse("0"),
							Allocatable: resource.MustParse("0"),
							Capacity:    resource.MustParse("0"),
						},
					},
				},
			}

			resp := &v1.ListPodResourcesResponse{
				PodResources: []*v1.PodResources{},
			}
			mockPodResClient.On("List", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("*v1.ListPodResourcesRequest")).Return(resp, nil)
			res, err := resMon.Scan(ResourceExcludeList{}) // no pods allocation
			So(err, ShouldBeNil)

			// TODO: reset memory

			sort.Slice(res, func(i, j int) bool {
				return res[i].Name < res[j].Name
			})
			for _, resource := range res {
				sort.Slice(resource.Costs, func(x, y int) bool {
					return resource.Costs[x].Name < resource.Costs[y].Name
				})
			}
			for _, resource := range res {
				sort.Slice(resource.Resources, func(x, y int) bool {
					return resource.Resources[x].Name < resource.Resources[y].Name
				})
			}
			log.Printf("result=%v", res)
			log.Printf("expected=%v", expected)
			log.Printf("diff=%s", cmp.Diff(res, expected))
			So(cmp.Equal(res, expected), ShouldBeTrue)
		})
	})

	Convey("When I aggregate the node resources fake data, no pod allocation and some reserved CPUs", t, func() {
		availRes := &v1.AllocatableResourcesResponse{
			Devices: allContainerDevices,
			// CPUId 0 and 1 are missing from the list below to simulate
			// that they are not allocatable CPUs (kube-reserved or system-reserved)
			CpuIds: []int64{
				2, 3, 4, 5, 6, 7, 8, 9, 10, 11,
				12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23,
			},
		}

		mockPodResClient := new(podres.MockPodResourcesListerClient)
		mockPodResClient.On("GetAllocatableResources", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("*v1.AllocatableResourcesRequest")).Return(availRes, nil)
		resMon, err := NewResourceMonitorWithTopology("TEST", &fakeTopo, mockPodResClient, Args{})
		So(err, ShouldBeNil)

		Convey("When aggregating resources", func() {
			expected := topologyv1alpha1.ZoneList{
				topologyv1alpha1.Zone{
					Name: "node-0",
					Type: "Node",
					Costs: topologyv1alpha1.CostList{
						topologyv1alpha1.CostInfo{
							Name:  "node-0",
							Value: 10,
						},
						topologyv1alpha1.CostInfo{
							Name:  "node-1",
							Value: 20,
						},
					},
					Resources: topologyv1alpha1.ResourceInfoList{
						topologyv1alpha1.ResourceInfo{
							Name:        "cpu",
							Available:   resource.MustParse("11"),
							Allocatable: resource.MustParse("11"),
							Capacity:    resource.MustParse("12"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "fake.io/net",
							Available:   resource.MustParse("4"),
							Allocatable: resource.MustParse("4"),
							Capacity:    resource.MustParse("4"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "memory",
							Available:   resource.MustParse("0"),
							Allocatable: resource.MustParse("0"),
							Capacity:    resource.MustParse("0"),
						},
					},
				},
				topologyv1alpha1.Zone{
					Name: "node-1",
					Type: "Node",
					Costs: topologyv1alpha1.CostList{
						topologyv1alpha1.CostInfo{
							Name:  "node-0",
							Value: 20,
						},
						topologyv1alpha1.CostInfo{
							Name:  "node-1",
							Value: 10,
						},
					},
					Resources: topologyv1alpha1.ResourceInfoList{
						topologyv1alpha1.ResourceInfo{
							Name:        "cpu",
							Available:   resource.MustParse("11"),
							Allocatable: resource.MustParse("11"),
							Capacity:    resource.MustParse("12"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "fake.io/gpu",
							Available:   resource.MustParse("1"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "fake.io/net",
							Available:   resource.MustParse("2"),
							Allocatable: resource.MustParse("2"),
							Capacity:    resource.MustParse("2"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "memory",
							Available:   resource.MustParse("0"),
							Allocatable: resource.MustParse("0"),
							Capacity:    resource.MustParse("0"),
						},
					},
				},
			}

			resp := &v1.ListPodResourcesResponse{
				PodResources: []*v1.PodResources{},
			}
			mockPodResClient.On("List", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("*v1.ListPodResourcesRequest")).Return(resp, nil)
			res, err := resMon.Scan(ResourceExcludeList{}) // no pods allocation
			So(err, ShouldBeNil)

			sort.Slice(res, func(i, j int) bool {
				return res[i].Name < res[j].Name
			})
			for _, resource := range res {
				sort.Slice(resource.Costs, func(x, y int) bool {
					return resource.Costs[x].Name < resource.Costs[y].Name
				})
			}
			for _, resource := range res {
				sort.Slice(resource.Resources, func(x, y int) bool {
					return resource.Resources[x].Name < resource.Resources[y].Name
				})
			}
			log.Printf("result=%v", res)
			log.Printf("expected=%v", expected)
			log.Printf("diff=%s", cmp.Diff(res, expected))
			So(cmp.Equal(res, expected), ShouldBeTrue)
		})
	})

	minimalContainerDevices := []*v1.ContainerDevices{
		{
			ResourceName: "fake.io/net",
			DeviceIds:    []string{"netAAA"},
			Topology: &v1.TopologyInfo{
				Nodes: []*v1.NUMANode{
					{
						ID: 0,
					},
				},
			},
		},
		{
			ResourceName: "fake.io/resourceToBeExcluded",
			DeviceIds:    []string{"excludeMeA"},
			Topology: &v1.TopologyInfo{
				Nodes: []*v1.NUMANode{
					{
						ID: 0,
					},
				},
			},
		},
		{
			ResourceName: "fake.io/net",
			DeviceIds:    []string{"netBBB"},
			Topology: &v1.TopologyInfo{
				Nodes: []*v1.NUMANode{
					{
						ID: 1,
					},
				},
			},
		},
		{
			ResourceName: "fake.io/gpu",
			DeviceIds:    []string{"gpuAAA"},
			Topology: &v1.TopologyInfo{
				Nodes: []*v1.NUMANode{
					{
						ID: 1,
					},
				},
			},
		},
		{
			ResourceName: "fake.io/resourceToBeExcluded",
			DeviceIds:    []string{"excludeMeB"},
			Topology: &v1.TopologyInfo{
				Nodes: []*v1.NUMANode{
					{
						ID: 1,
					},
				},
			},
		},
	}

	Convey("When I aggregate the node resources fake data and some pod allocation", t, func() {
		allocRes := &v1.AllocatableResourcesResponse{
			Devices: minimalContainerDevices,
			CpuIds: []int64{
				0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11,
				12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23,
			},
		}

		mockPodResClient := new(podres.MockPodResourcesListerClient)
		mockPodResClient.On("GetAllocatableResources", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("*v1.AllocatableResourcesRequest")).Return(allocRes, nil)
		resMon, err := NewResourceMonitorWithTopology("TEST", &fakeTopo, mockPodResClient, Args{})
		So(err, ShouldBeNil)

		Convey("When aggregating resources", func() {
			resp := &v1.ListPodResourcesResponse{
				PodResources: []*v1.PodResources{
					{
						Name:      "test-pod-0",
						Namespace: "default",
						Containers: []*v1.ContainerResources{
							{
								Name:   "test-cnt-0",
								CpuIds: []int64{5, 7},
								Devices: []*v1.ContainerDevices{
									{
										ResourceName: "fake.io/net",
										DeviceIds:    []string{"netBBB"},
										Topology: &v1.TopologyInfo{
											Nodes: []*v1.NUMANode{
												{
													ID: 1,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}

			expected := topologyv1alpha1.ZoneList{
				topologyv1alpha1.Zone{
					Name: "node-0",
					Type: "Node",
					Costs: topologyv1alpha1.CostList{
						topologyv1alpha1.CostInfo{
							Name:  "node-0",
							Value: 10,
						},
						topologyv1alpha1.CostInfo{
							Name:  "node-1",
							Value: 20,
						},
					},
					Resources: topologyv1alpha1.ResourceInfoList{
						topologyv1alpha1.ResourceInfo{
							Name:        "cpu",
							Available:   resource.MustParse("12"),
							Allocatable: resource.MustParse("12"),
							Capacity:    resource.MustParse("12"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "fake.io/net",
							Available:   resource.MustParse("1"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "fake.io/resourceToBeExcluded",
							Available:   resource.MustParse("1"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "memory",
							Available:   resource.MustParse("0"),
							Allocatable: resource.MustParse("0"),
							Capacity:    resource.MustParse("0"),
						},
					},
				},
				topologyv1alpha1.Zone{
					Name: "node-1",
					Type: "Node",
					Costs: topologyv1alpha1.CostList{
						topologyv1alpha1.CostInfo{
							Name:  "node-0",
							Value: 20,
						},
						topologyv1alpha1.CostInfo{
							Name:  "node-1",
							Value: 10,
						},
					},
					Resources: topologyv1alpha1.ResourceInfoList{
						topologyv1alpha1.ResourceInfo{
							Name:        "cpu",
							Available:   resource.MustParse("10"),
							Allocatable: resource.MustParse("12"),
							Capacity:    resource.MustParse("12"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "fake.io/gpu",
							Available:   resource.MustParse("1"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "fake.io/net",
							Available:   resource.MustParse("0"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "fake.io/resourceToBeExcluded",
							Available:   resource.MustParse("1"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "memory",
							Available:   resource.MustParse("0"),
							Allocatable: resource.MustParse("0"),
							Capacity:    resource.MustParse("0"),
						},
					},
				},
			}

			excludeList := ResourceExcludeList{
				map[string][]string{
					"*": {
						"fake.io/resourceToBeExcluded",
					},
				},
			}

			mockPodResClient.On("List", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("*v1.ListPodResourcesRequest")).Return(resp, nil)
			res, err := resMon.Scan(excludeList)
			So(err, ShouldBeNil)
			// Check if resources were excluded correctly
			for _, zone := range res {
				for _, resource := range zone.Resources {
					assert.NotEqual(t, resource.Name, "fake.io/resourceToBeExcluded", "fake.io/resourceToBeExcluded has to be excluded")
				}
			}

			// Add devices after they have been removed by the exclude list
			for i := range res {
				res[i].Resources = append(res[i].Resources, topologyv1alpha1.ResourceInfo{
					Name:        "fake.io/resourceToBeExcluded",
					Available:   resource.MustParse("1"),
					Allocatable: resource.MustParse("1"),
					Capacity:    resource.MustParse("1"),
				})
			}

			sort.Slice(res, func(i, j int) bool {
				return res[i].Name < res[j].Name
			})
			for _, resource := range res {
				sort.Slice(resource.Costs, func(x, y int) bool {
					return resource.Costs[x].Name < resource.Costs[y].Name
				})
			}
			for _, resource := range res {
				sort.Slice(resource.Resources, func(x, y int) bool {
					return resource.Resources[x].Name < resource.Resources[y].Name
				})
			}
			log.Printf("result=%v", res)
			log.Printf("expected=%v", expected)
			log.Printf("diff=%s", cmp.Diff(res, expected))
			So(cmp.Equal(res, expected), ShouldBeTrue)
		})
	})

	Convey("When I aggregate the node resources fake data, some pod allocation and some reserved CPUs", t, func() {
		allocRes := &v1.AllocatableResourcesResponse{
			Devices: minimalContainerDevices,
			// CPUId 0 is missing from the list below to simulate
			// that it not allocatable (kube-reserved or system-reserved)
			CpuIds: []int64{
				1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11,
				12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23,
			},
		}

		mockPodResClient := new(podres.MockPodResourcesListerClient)
		mockPodResClient.On("GetAllocatableResources", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("*v1.AllocatableResourcesRequest")).Return(allocRes, nil)
		resMon, err := NewResourceMonitorWithTopology("TEST", &fakeTopo, mockPodResClient, Args{})
		So(err, ShouldBeNil)

		Convey("When aggregating resources", func() {
			resp := &v1.ListPodResourcesResponse{
				PodResources: []*v1.PodResources{
					{
						Name:      "test-pod-0",
						Namespace: "default",
						Containers: []*v1.ContainerResources{
							{
								Name:   "test-cnt-0",
								CpuIds: []int64{5, 7},
								Devices: []*v1.ContainerDevices{
									{
										ResourceName: "fake.io/net",
										DeviceIds:    []string{"netBBB"},
										Topology: &v1.TopologyInfo{
											Nodes: []*v1.NUMANode{
												{
													ID: 1,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}

			expected := topologyv1alpha1.ZoneList{
				topologyv1alpha1.Zone{
					Name: "node-0",
					Type: "Node",
					Costs: topologyv1alpha1.CostList{
						topologyv1alpha1.CostInfo{
							Name:  "node-0",
							Value: 10,
						},
						topologyv1alpha1.CostInfo{
							Name:  "node-1",
							Value: 20,
						},
					},
					Resources: topologyv1alpha1.ResourceInfoList{
						topologyv1alpha1.ResourceInfo{
							Name:        "cpu",
							Available:   resource.MustParse("11"),
							Allocatable: resource.MustParse("11"),
							Capacity:    resource.MustParse("12"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "fake.io/net",
							Available:   resource.MustParse("1"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "fake.io/resourceToBeExcluded",
							Available:   resource.MustParse("1"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "memory",
							Available:   resource.MustParse("0"),
							Allocatable: resource.MustParse("0"),
							Capacity:    resource.MustParse("0"),
						},
					},
				},
				topologyv1alpha1.Zone{
					Name: "node-1",
					Type: "Node",
					Costs: topologyv1alpha1.CostList{
						topologyv1alpha1.CostInfo{
							Name:  "node-0",
							Value: 20,
						},
						topologyv1alpha1.CostInfo{
							Name:  "node-1",
							Value: 10,
						},
					},
					Resources: topologyv1alpha1.ResourceInfoList{
						topologyv1alpha1.ResourceInfo{
							Name:        "cpu",
							Available:   resource.MustParse("10"),
							Allocatable: resource.MustParse("12"),
							Capacity:    resource.MustParse("12"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "fake.io/gpu",
							Available:   resource.MustParse("1"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "fake.io/net",
							Available:   resource.MustParse("0"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "fake.io/resourceToBeExcluded",
							Available:   resource.MustParse("1"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "memory",
							Available:   resource.MustParse("0"),
							Allocatable: resource.MustParse("0"),
							Capacity:    resource.MustParse("0"),
						},
					},
				},
			}

			excludeList := ResourceExcludeList{
				map[string][]string{
					"*": {
						"fake.io/resourceToBeExcluded",
					},
				},
			}

			mockPodResClient.On("List", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("*v1.ListPodResourcesRequest")).Return(resp, nil)
			res, err := resMon.Scan(excludeList)
			So(err, ShouldBeNil)
			// Check if resources were excluded correctly
			for _, zone := range res {
				for _, resource := range zone.Resources {
					assert.NotEqual(t, resource.Name, "fake.io/resourceToBeExcluded", "fake.io/resourceToBeExcluded has to be excluded")
				}
			}

			// Add devices after they have been removed by the exclude list
			for i := range res {
				res[i].Resources = append(res[i].Resources, topologyv1alpha1.ResourceInfo{
					Name:        "fake.io/resourceToBeExcluded",
					Available:   resource.MustParse("1"),
					Allocatable: resource.MustParse("1"),
					Capacity:    resource.MustParse("1"),
				})
			}

			sort.Slice(res, func(i, j int) bool {
				return res[i].Name < res[j].Name
			})
			for _, resource := range res {
				sort.Slice(resource.Costs, func(x, y int) bool {
					return resource.Costs[x].Name < resource.Costs[y].Name
				})
			}
			for _, resource := range res {
				sort.Slice(resource.Resources, func(x, y int) bool {
					return resource.Resources[x].Name < resource.Resources[y].Name
				})
			}
			log.Printf("result=%v", res)
			log.Printf("expected=%v", expected)
			log.Printf("diff=%s", cmp.Diff(res, expected))
			So(cmp.Equal(res, expected), ShouldBeTrue)
		})
	})

	Convey("When I aggregate the node resources fake data and some pod allocation, with refresh allocation", t, func() {
		mockPodResClient := new(podres.MockPodResourcesListerClient)
		resMon, err := NewResourceMonitorWithTopology("TEST", &fakeTopo, mockPodResClient, Args{RefreshNodeResources: true})
		So(err, ShouldBeNil)

		Convey("When aggregating resources", func() {
			allocRes := &v1.AllocatableResourcesResponse{
				Devices: []*v1.ContainerDevices{
					{
						ResourceName: "fake.io/net",
						DeviceIds:    []string{"netAAA"},
						Topology: &v1.TopologyInfo{
							Nodes: []*v1.NUMANode{
								{
									ID: 0,
								},
							},
						},
					},
					{
						ResourceName: "fake.io/resourceToBeExcluded",
						DeviceIds:    []string{"excludeMeA"},
						Topology: &v1.TopologyInfo{
							Nodes: []*v1.NUMANode{
								{
									ID: 0,
								},
							},
						},
					},
					{
						ResourceName: "fake.io/net",
						DeviceIds:    []string{"netBBB"},
						Topology: &v1.TopologyInfo{
							Nodes: []*v1.NUMANode{
								{
									ID: 1,
								},
							},
						},
					},
					{
						ResourceName: "fake.io/gpu",
						DeviceIds:    []string{"gpuAAA"},
						Topology: &v1.TopologyInfo{
							Nodes: []*v1.NUMANode{
								{
									ID: 1,
								},
							},
						},
					},
					{
						ResourceName: "fake.io/resourceToBeExcluded",
						DeviceIds:    []string{"excludeMeB"},
						Topology: &v1.TopologyInfo{
							Nodes: []*v1.NUMANode{
								{
									ID: 1,
								},
							},
						},
					},
				},
				CpuIds: []int64{
					0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11,
					12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23,
				},
			}

			resp := &v1.ListPodResourcesResponse{
				PodResources: []*v1.PodResources{
					{
						Name:      "test-pod-0",
						Namespace: "default",
						Containers: []*v1.ContainerResources{
							{
								Name:   "test-cnt-0",
								CpuIds: []int64{5, 7},
								Devices: []*v1.ContainerDevices{
									{
										ResourceName: "fake.io/net",
										DeviceIds:    []string{"netBBB"},
										Topology: &v1.TopologyInfo{
											Nodes: []*v1.NUMANode{
												{
													ID: 1,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}

			expected := topologyv1alpha1.ZoneList{
				topologyv1alpha1.Zone{
					Name: "node-0",
					Type: "Node",
					Costs: topologyv1alpha1.CostList{
						topologyv1alpha1.CostInfo{
							Name:  "node-0",
							Value: 10,
						},
						topologyv1alpha1.CostInfo{
							Name:  "node-1",
							Value: 20,
						},
					},
					Resources: topologyv1alpha1.ResourceInfoList{
						topologyv1alpha1.ResourceInfo{
							Name:        "cpu",
							Available:   resource.MustParse("12"),
							Allocatable: resource.MustParse("12"),
							Capacity:    resource.MustParse("12"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "fake.io/net",
							Available:   resource.MustParse("1"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "fake.io/resourceToBeExcluded",
							Available:   resource.MustParse("1"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "memory",
							Available:   resource.MustParse("0"),
							Allocatable: resource.MustParse("0"),
							Capacity:    resource.MustParse("16638164992"),
						},
					},
				},
				topologyv1alpha1.Zone{
					Name: "node-1",
					Type: "Node",
					Costs: topologyv1alpha1.CostList{
						topologyv1alpha1.CostInfo{
							Name:  "node-0",
							Value: 20,
						},
						topologyv1alpha1.CostInfo{
							Name:  "node-1",
							Value: 10,
						},
					},
					Resources: topologyv1alpha1.ResourceInfoList{
						topologyv1alpha1.ResourceInfo{
							Name:        "cpu",
							Available:   resource.MustParse("10"),
							Allocatable: resource.MustParse("12"),
							Capacity:    resource.MustParse("12"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "fake.io/gpu",
							Available:   resource.MustParse("1"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "fake.io/net",
							Available:   resource.MustParse("0"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "fake.io/resourceToBeExcluded",
							Available:   resource.MustParse("1"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
						topologyv1alpha1.ResourceInfo{
							Name:        "memory",
							Available:   resource.MustParse("0"),
							Allocatable: resource.MustParse("0"),
							Capacity:    resource.MustParse("16638164992"),
						},
					},
				},
			}

			excludeList := ResourceExcludeList{
				map[string][]string{
					"*": {
						"fake.io/resourceToBeExcluded",
					},
				},
			}

			mockPodResClient.On("GetAllocatableResources", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("*v1.AllocatableResourcesRequest")).Return(allocRes, nil)
			mockPodResClient.On("List", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("*v1.ListPodResourcesRequest")).Return(resp, nil)
			res, err := resMon.Scan(excludeList)
			So(err, ShouldBeNil)
			// Check if resources were excluded correctly
			for _, zone := range res {
				for _, resource := range zone.Resources {
					assert.NotEqual(t, resource.Name, "fake.io/resourceToBeExcluded", "fake.io/resourceToBeExcluded has to be excluded")
				}
			}

			// Add devices after they have been removed by the exclude list
			for i := range res {
				res[i].Resources = append(res[i].Resources, topologyv1alpha1.ResourceInfo{
					Name:        "fake.io/resourceToBeExcluded",
					Available:   resource.MustParse("1"),
					Allocatable: resource.MustParse("1"),
					Capacity:    resource.MustParse("1"),
				})
			}

			sort.Slice(res, func(i, j int) bool {
				return res[i].Name < res[j].Name
			})
			for _, resource := range res {
				sort.Slice(resource.Costs, func(x, y int) bool {
					return resource.Costs[x].Name < resource.Costs[y].Name
				})
			}
			for _, resource := range res {
				sort.Slice(resource.Resources, func(x, y int) bool {
					return resource.Resources[x].Name < resource.Resources[y].Name
				})
			}
			log.Printf("result=%v", res)
			log.Printf("expected=%v", expected)
			log.Printf("diff=%s", cmp.Diff(res, expected))
			So(cmp.Equal(res, expected), ShouldBeTrue)
		})
	})

}

func getExpectedCoreToNodeMap() map[int]int {
	return map[int]int{
		0:  0,
		2:  0,
		4:  0,
		6:  0,
		8:  0,
		10: 0,
		12: 0,
		14: 0,
		16: 0,
		18: 0,
		20: 0,
		22: 0,
		1:  1,
		3:  1,
		5:  1,
		7:  1,
		9:  1,
		11: 1,
		13: 1,
		15: 1,
		17: 1,
		19: 1,
		21: 1,
		23: 1,
	}
}

// ghwc topology -f json
var testTopology string = `{
    "nodes": [
      {
        "id": 0,
        "cores": [
          {
            "id": 0,
            "index": 0,
            "total_threads": 2,
            "logical_processors": [
              0,
              12
            ]
          },
          {
            "id": 10,
            "index": 1,
            "total_threads": 2,
            "logical_processors": [
              10,
              22
            ]
          },
          {
            "id": 1,
            "index": 2,
            "total_threads": 2,
            "logical_processors": [
              14,
              2
            ]
          },
          {
            "id": 2,
            "index": 3,
            "total_threads": 2,
            "logical_processors": [
              16,
              4
            ]
          },
          {
            "id": 8,
            "index": 4,
            "total_threads": 2,
            "logical_processors": [
              18,
              6
            ]
          },
          {
            "id": 9,
            "index": 5,
            "total_threads": 2,
            "logical_processors": [
              20,
              8
            ]
          }
        ],
        "distances": [
          10,
          20
        ]
      },
      {
        "id": 1,
        "cores": [
          {
            "id": 0,
            "index": 0,
            "total_threads": 2,
            "logical_processors": [
              1,
              13
            ]
          },
          {
            "id": 10,
            "index": 1,
            "total_threads": 2,
            "logical_processors": [
              11,
              23
            ]
          },
          {
            "id": 1,
            "index": 2,
            "total_threads": 2,
            "logical_processors": [
              15,
              3
            ]
          },
          {
            "id": 2,
            "index": 3,
            "total_threads": 2,
            "logical_processors": [
              17,
              5
            ]
          },
          {
            "id": 8,
            "index": 4,
            "total_threads": 2,
            "logical_processors": [
              19,
              7
            ]
          },
          {
            "id": 9,
            "index": 5,
            "total_threads": 2,
            "logical_processors": [
              21,
              9
            ]
          }
        ],
        "distances": [
          20,
          10
        ]
      }
    ]
}`
