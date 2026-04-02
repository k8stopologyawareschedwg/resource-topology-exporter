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
	"errors"
	"log"
	"sort"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/klog/v2"
	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"
	v1 "k8s.io/kubelet/pkg/apis/podresources/v1"

	cmp "github.com/google/go-cmp/cmp"
	ghwtopology "github.com/jaypipes/ghw/pkg/topology"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/protobuf/testing/protocmp"

	topologyv1alpha2 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha2"
	"github.com/k8stopologyawareschedwg/numaplacement"
	"github.com/k8stopologyawareschedwg/podfingerprint"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres"
)

func TestMakeCoreIDToNodeIDMap(t *testing.T) {
	fakeTopo := ghwtopology.Info{}
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
				Size:       1024,
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
				Size:       1024,
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
				Size:       1024,
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
		VL := klog.V(999) // so high that means "never" in practice
		res := NormalizeContainerDevices(VL, availRes.GetDevices(), availRes.GetMemory(), availRes.GetCpuIds(), coreIDToNodeIDMap)
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
		log.Printf("diff=%s", cmp.Diff(res, expected, protocmp.Transform()))
		So(cmp.Equal(res, expected, protocmp.Transform()), ShouldBeTrue)
	})
}

// TODO: add testcase for
// - a pod with non-integral CPUs and devices, we need to not decrement the CPUs but do that for devices.

func TestResourcesScan(t *testing.T) {
	fakeTopo := ghwtopology.Info{}
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
		resMon, err := NewResourceMonitor(Handle{PodResCli: mockPodResClient}, Args{}, WithNodeName("TEST"), WithTopology(&fakeTopo), WithK8sClient(fake.NewSimpleClientset()))
		So(err, ShouldBeNil)

		Convey("When aggregating resources", func() {
			expected := topologyv1alpha2.ZoneList{
				topologyv1alpha2.Zone{
					Name: "node-0",
					Type: "Node",
					Costs: topologyv1alpha2.CostList{
						topologyv1alpha2.CostInfo{
							Name:  "node-0",
							Value: 10,
						},
						topologyv1alpha2.CostInfo{
							Name:  "node-1",
							Value: 20,
						},
					},
					Resources: topologyv1alpha2.ResourceInfoList{
						topologyv1alpha2.ResourceInfo{
							Name:        "cpu",
							Available:   resource.MustParse("12"),
							Allocatable: resource.MustParse("12"),
							Capacity:    resource.MustParse("12"),
						},
						topologyv1alpha2.ResourceInfo{
							Name:        "fake.io/net",
							Available:   resource.MustParse("4"),
							Allocatable: resource.MustParse("4"),
							Capacity:    resource.MustParse("4"),
						},
					},
				},
				topologyv1alpha2.Zone{
					Name: "node-1",
					Type: "Node",
					Costs: topologyv1alpha2.CostList{
						topologyv1alpha2.CostInfo{
							Name:  "node-0",
							Value: 20,
						},
						topologyv1alpha2.CostInfo{
							Name:  "node-1",
							Value: 10,
						},
					},
					Resources: topologyv1alpha2.ResourceInfoList{
						topologyv1alpha2.ResourceInfo{
							Name:        "cpu",
							Available:   resource.MustParse("12"),
							Allocatable: resource.MustParse("12"),
							Capacity:    resource.MustParse("12"),
						},
						topologyv1alpha2.ResourceInfo{
							Name:        "fake.io/gpu",
							Available:   resource.MustParse("1"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
						topologyv1alpha2.ResourceInfo{
							Name:        "fake.io/net",
							Available:   resource.MustParse("2"),
							Allocatable: resource.MustParse("2"),
							Capacity:    resource.MustParse("2"),
						},
					},
				},
			}

			resp := &v1.ListPodResourcesResponse{
				PodResources: []*v1.PodResources{},
			}
			mockPodResClient.On("List", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("*v1.ListPodResourcesRequest")).Return(resp, nil)
			scanRes, err := resMon.Scan(ResourceExclude{}) // no pods allocation
			So(err, ShouldBeNil)

			res := scanRes.SortedZones()
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
		resMon, err := NewResourceMonitor(Handle{PodResCli: mockPodResClient}, Args{}, WithNodeName("TEST"), WithTopology(&fakeTopo), WithK8sClient(fake.NewSimpleClientset()))
		So(err, ShouldBeNil)

		Convey("When aggregating resources", func() {
			expected := topologyv1alpha2.ZoneList{
				topologyv1alpha2.Zone{
					Name: "node-0",
					Type: "Node",
					Costs: topologyv1alpha2.CostList{
						topologyv1alpha2.CostInfo{
							Name:  "node-0",
							Value: 10,
						},
						topologyv1alpha2.CostInfo{
							Name:  "node-1",
							Value: 20,
						},
					},
					Resources: topologyv1alpha2.ResourceInfoList{
						topologyv1alpha2.ResourceInfo{
							Name:        "cpu",
							Available:   resource.MustParse("11"),
							Allocatable: resource.MustParse("11"),
							Capacity:    resource.MustParse("12"),
						},
						topologyv1alpha2.ResourceInfo{
							Name:        "fake.io/net",
							Available:   resource.MustParse("4"),
							Allocatable: resource.MustParse("4"),
							Capacity:    resource.MustParse("4"),
						},
					},
				},
				topologyv1alpha2.Zone{
					Name: "node-1",
					Type: "Node",
					Costs: topologyv1alpha2.CostList{
						topologyv1alpha2.CostInfo{
							Name:  "node-0",
							Value: 20,
						},
						topologyv1alpha2.CostInfo{
							Name:  "node-1",
							Value: 10,
						},
					},
					Resources: topologyv1alpha2.ResourceInfoList{
						topologyv1alpha2.ResourceInfo{
							Name:        "cpu",
							Available:   resource.MustParse("11"),
							Allocatable: resource.MustParse("11"),
							Capacity:    resource.MustParse("12"),
						},
						topologyv1alpha2.ResourceInfo{
							Name:        "fake.io/gpu",
							Available:   resource.MustParse("1"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
						topologyv1alpha2.ResourceInfo{
							Name:        "fake.io/net",
							Available:   resource.MustParse("2"),
							Allocatable: resource.MustParse("2"),
							Capacity:    resource.MustParse("2"),
						},
					},
				},
			}

			resp := &v1.ListPodResourcesResponse{
				PodResources: []*v1.PodResources{},
			}
			mockPodResClient.On("List", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("*v1.ListPodResourcesRequest")).Return(resp, nil)
			scanRes, err := resMon.Scan(ResourceExclude{}) // no pods allocation
			So(err, ShouldBeNil)

			res := scanRes.SortedZones()
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
		resMon, err := NewResourceMonitor(Handle{PodResCli: mockPodResClient}, Args{}, WithNodeName("TEST"), WithTopology(&fakeTopo), WithK8sClient(fake.NewSimpleClientset()))
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

			expected := topologyv1alpha2.ZoneList{
				topologyv1alpha2.Zone{
					Name: "node-0",
					Type: "Node",
					Costs: topologyv1alpha2.CostList{
						topologyv1alpha2.CostInfo{
							Name:  "node-0",
							Value: 10,
						},
						topologyv1alpha2.CostInfo{
							Name:  "node-1",
							Value: 20,
						},
					},
					Resources: topologyv1alpha2.ResourceInfoList{
						topologyv1alpha2.ResourceInfo{
							Name:        "cpu",
							Available:   resource.MustParse("12"),
							Allocatable: resource.MustParse("12"),
							Capacity:    resource.MustParse("12"),
						},
						topologyv1alpha2.ResourceInfo{
							Name:        "fake.io/net",
							Available:   resource.MustParse("1"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
						topologyv1alpha2.ResourceInfo{
							Name:        "fake.io/resourceToBeExcluded",
							Available:   resource.MustParse("1"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
					},
				},
				topologyv1alpha2.Zone{
					Name: "node-1",
					Type: "Node",
					Costs: topologyv1alpha2.CostList{
						topologyv1alpha2.CostInfo{
							Name:  "node-0",
							Value: 20,
						},
						topologyv1alpha2.CostInfo{
							Name:  "node-1",
							Value: 10,
						},
					},
					Resources: topologyv1alpha2.ResourceInfoList{
						topologyv1alpha2.ResourceInfo{
							Name:        "cpu",
							Available:   resource.MustParse("10"),
							Allocatable: resource.MustParse("12"),
							Capacity:    resource.MustParse("12"),
						},
						topologyv1alpha2.ResourceInfo{
							Name:        "fake.io/gpu",
							Available:   resource.MustParse("1"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
						topologyv1alpha2.ResourceInfo{
							Name:        "fake.io/net",
							Available:   resource.MustParse("0"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
						topologyv1alpha2.ResourceInfo{
							Name:        "fake.io/resourceToBeExcluded",
							Available:   resource.MustParse("1"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
					},
				},
			}

			excludeList := ResourceExclude{
				"*": {
					"fake.io/resourceToBeExcluded",
				},
			}

			mockPodResClient.On("List", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("*v1.ListPodResourcesRequest")).Return(resp, nil)
			scanRes, err := resMon.Scan(excludeList)
			So(err, ShouldBeNil)

			res := scanRes.Zones.DeepCopy()

			// Check if resources were excluded correctly
			for _, zone := range res {
				for _, resource := range zone.Resources {
					assert.NotEqual(t, resource.Name, "fake.io/resourceToBeExcluded", "fake.io/resourceToBeExcluded has to be excluded")
				}
			}

			// Add devices after they have been removed by the exclude list
			for i := range res {
				res[i].Resources = append(res[i].Resources, topologyv1alpha2.ResourceInfo{
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
		resMon, err := NewResourceMonitor(Handle{PodResCli: mockPodResClient}, Args{}, WithNodeName("TEST"), WithTopology(&fakeTopo), WithK8sClient(fake.NewSimpleClientset()))
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

			expected := topologyv1alpha2.ZoneList{
				topologyv1alpha2.Zone{
					Name: "node-0",
					Type: "Node",
					Costs: topologyv1alpha2.CostList{
						topologyv1alpha2.CostInfo{
							Name:  "node-0",
							Value: 10,
						},
						topologyv1alpha2.CostInfo{
							Name:  "node-1",
							Value: 20,
						},
					},
					Resources: topologyv1alpha2.ResourceInfoList{
						topologyv1alpha2.ResourceInfo{
							Name:        "cpu",
							Available:   resource.MustParse("11"),
							Allocatable: resource.MustParse("11"),
							Capacity:    resource.MustParse("12"),
						},
						topologyv1alpha2.ResourceInfo{
							Name:        "fake.io/net",
							Available:   resource.MustParse("1"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
						topologyv1alpha2.ResourceInfo{
							Name:        "fake.io/resourceToBeExcluded",
							Available:   resource.MustParse("1"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
					},
				},
				topologyv1alpha2.Zone{
					Name: "node-1",
					Type: "Node",
					Costs: topologyv1alpha2.CostList{
						topologyv1alpha2.CostInfo{
							Name:  "node-0",
							Value: 20,
						},
						topologyv1alpha2.CostInfo{
							Name:  "node-1",
							Value: 10,
						},
					},
					Resources: topologyv1alpha2.ResourceInfoList{
						topologyv1alpha2.ResourceInfo{
							Name:        "cpu",
							Available:   resource.MustParse("10"),
							Allocatable: resource.MustParse("12"),
							Capacity:    resource.MustParse("12"),
						},
						topologyv1alpha2.ResourceInfo{
							Name:        "fake.io/gpu",
							Available:   resource.MustParse("1"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
						topologyv1alpha2.ResourceInfo{
							Name:        "fake.io/net",
							Available:   resource.MustParse("0"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
						topologyv1alpha2.ResourceInfo{
							Name:        "fake.io/resourceToBeExcluded",
							Available:   resource.MustParse("1"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
					},
				},
			}

			excludeList := ResourceExclude{
				"*": {
					"fake.io/resourceToBeExcluded",
				},
			}

			mockPodResClient.On("List", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("*v1.ListPodResourcesRequest")).Return(resp, nil)
			scanRes, err := resMon.Scan(excludeList)
			So(err, ShouldBeNil)

			res := scanRes.Zones.DeepCopy()
			// Check if resources were excluded correctly
			for _, zone := range res {
				for _, resource := range zone.Resources {
					assert.NotEqual(t, resource.Name, "fake.io/resourceToBeExcluded", "fake.io/resourceToBeExcluded has to be excluded")
				}
			}

			// Add devices after they have been removed by the exclude list
			for i := range res {
				res[i].Resources = append(res[i].Resources, topologyv1alpha2.ResourceInfo{
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
		mockPodResClient := new(podres.MockPodResourcesListerClient)
		mockPodResClient.On("GetAllocatableResources", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("*v1.AllocatableResourcesRequest")).Return(allocRes, nil)
		resMon, err := NewResourceMonitor(Handle{PodResCli: mockPodResClient}, Args{RefreshNodeResources: true}, WithNodeName("TEST"), WithTopology(&fakeTopo), WithK8sClient(fake.NewSimpleClientset()))
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

			expected := topologyv1alpha2.ZoneList{
				topologyv1alpha2.Zone{
					Name: "node-0",
					Type: "Node",
					Costs: topologyv1alpha2.CostList{
						topologyv1alpha2.CostInfo{
							Name:  "node-0",
							Value: 10,
						},
						topologyv1alpha2.CostInfo{
							Name:  "node-1",
							Value: 20,
						},
					},
					Resources: topologyv1alpha2.ResourceInfoList{
						topologyv1alpha2.ResourceInfo{
							Name:        "cpu",
							Available:   resource.MustParse("12"),
							Allocatable: resource.MustParse("12"),
							Capacity:    resource.MustParse("12"),
						},
						topologyv1alpha2.ResourceInfo{
							Name:        "fake.io/net",
							Available:   resource.MustParse("1"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
						topologyv1alpha2.ResourceInfo{
							Name:        "fake.io/resourceToBeExcluded",
							Available:   resource.MustParse("1"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
					},
				},
				topologyv1alpha2.Zone{
					Name: "node-1",
					Type: "Node",
					Costs: topologyv1alpha2.CostList{
						topologyv1alpha2.CostInfo{
							Name:  "node-0",
							Value: 20,
						},
						topologyv1alpha2.CostInfo{
							Name:  "node-1",
							Value: 10,
						},
					},
					Resources: topologyv1alpha2.ResourceInfoList{
						topologyv1alpha2.ResourceInfo{
							Name:        "cpu",
							Available:   resource.MustParse("10"),
							Allocatable: resource.MustParse("12"),
							Capacity:    resource.MustParse("12"),
						},
						topologyv1alpha2.ResourceInfo{
							Name:        "fake.io/gpu",
							Available:   resource.MustParse("1"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
						topologyv1alpha2.ResourceInfo{
							Name:        "fake.io/net",
							Available:   resource.MustParse("0"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
						topologyv1alpha2.ResourceInfo{
							Name:        "fake.io/resourceToBeExcluded",
							Available:   resource.MustParse("1"),
							Allocatable: resource.MustParse("1"),
							Capacity:    resource.MustParse("1"),
						},
					},
				},
			}

			excludeList := ResourceExclude{
				"*": {
					"fake.io/resourceToBeExcluded",
				},
			}

			mockPodResClient.On("List", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("*v1.ListPodResourcesRequest")).Return(resp, nil)
			scanRes, err := resMon.Scan(excludeList)
			So(err, ShouldBeNil)

			res := scanRes.Zones.DeepCopy()
			// Check if resources were excluded correctly
			for _, zone := range res {
				for _, resource := range zone.Resources {
					assert.NotEqual(t, resource.Name, "fake.io/resourceToBeExcluded", "fake.io/resourceToBeExcluded has to be excluded")
				}
			}

			// Add devices after they have been removed by the exclude list
			for i := range res {
				res[i].Resources = append(res[i].Resources, topologyv1alpha2.ResourceInfo{
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

	Convey("When I aggregate the node resources fake data and some pod allocation, with pod fingerprinting enabled", t, func() {
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
		resMon, err := NewResourceMonitor(Handle{PodResCli: mockPodResClient}, Args{PodSetFingerprint: true}, WithNodeName("TEST"), WithTopology(&fakeTopo), WithK8sClient(fake.NewSimpleClientset()))
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

			mockPodResClient.On("GetAllocatableResources", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("*v1.AllocatableResourcesRequest")).Return(allocRes, nil)
			mockPodResClient.On("List", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("*v1.ListPodResourcesRequest")).Return(resp, nil)
			scanRes, err := resMon.Scan(ResourceExclude{})

			expectedFP := "pfp0v001fe53c4dbd2c5f4a0" // pre-computed and validated manually
			fp, ok := scanRes.Annotations[podfingerprint.Annotation]
			So(ok, ShouldBeTrue)
			log.Printf("FP %q expected %q", fp, expectedFP)
			So(cmp.Equal(fp, expectedFP), ShouldBeTrue)

			So(err, ShouldBeNil)
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

func TestEncodeContainerAffinities(t *testing.T) {
	twoNUMANodesTopo := func() *ghwtopology.Info {
		return &ghwtopology.Info{
			Nodes: []*ghwtopology.Node{
				{ID: 0},
				{ID: 1},
			},
		}
	}
	t.Run("skips_when_topology_manager_policy_is_not_single_numa_node", func(t *testing.T) {
		rm := &resourceMonitor{
			args: Args{TopologyManagerPolicy: "restricted"},
			topo: twoNUMANodesTopo(),
		}
		podRes := []*podresourcesapi.PodResources{
			{
				Namespace: "ns",
				Name:      "pod",
				Containers: []*podresourcesapi.ContainerResources{
					{Name: "c", CpuIds: []int64{0}},
				},
			},
		}
		got, err := rm.encodeContainerAffinities(podRes)
		assert.NoError(t, err)
		assert.Equal(t, numaplacement.Payload{}, got)
	})

	t.Run("skips_when_no_pod_resources", func(t *testing.T) {
		rm := &resourceMonitor{
			args: Args{TopologyManagerPolicy: TMPolicySingleNUMANode},
			topo: twoNUMANodesTopo(),
		}
		got, err := rm.encodeContainerAffinities(nil)
		assert.NoError(t, err)
		assert.Equal(t, numaplacement.Payload{}, got)

		got, err = rm.encodeContainerAffinities([]*podresourcesapi.PodResources{})
		assert.NoError(t, err)
		assert.Equal(t, numaplacement.Payload{}, got)
	})

	t.Run("new_encoder_fails_when_topology_has_zero_numa_nodes", func(t *testing.T) {
		rm := &resourceMonitor{
			args: Args{TopologyManagerPolicy: TMPolicySingleNUMANode},
			topo: &ghwtopology.Info{Nodes: []*ghwtopology.Node{}},
		}
		podRes := []*podresourcesapi.PodResources{
			{
				Namespace: "ns",
				Name:      "pod",
				Containers: []*podresourcesapi.ContainerResources{
					{Name: "c", CpuIds: []int64{0}},
				},
			},
		}
		_, err := rm.encodeContainerAffinities(podRes)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, numaplacement.ErrInconsistentNUMANodes))
	})

	t.Run("skips_containers_that_do_not_pass_verify", func(t *testing.T) {
		rm := &resourceMonitor{
			args:              Args{TopologyManagerPolicy: TMPolicySingleNUMANode},
			topo:              twoNUMANodesTopo(),
			coreIDToNodeIDMap: map[int]int{0: 0},
		}
		podRes := []*podresourcesapi.PodResources{
			{
				Namespace: "ns",
				Name:      "pod",
				Containers: []*podresourcesapi.ContainerResources{
					{Name: "empty"},
				},
			},
		}
		got, err := rm.encodeContainerAffinities(podRes)
		assert.NoError(t, err)
		want := numaplacement.Payload{
			Containers:     0,
			NUMANodes:      2,
			BusiestNode:    0,
			VectorEncoding: numaplacement.VectorEncodingLEB89,
			Vectors:        map[int]string{},
		}
		assert.Empty(t, cmp.Diff(want, got))
	})

	t.Run("skips_when_find_numa_placement_errors", func(t *testing.T) {
		rm := &resourceMonitor{
			args:              Args{TopologyManagerPolicy: TMPolicySingleNUMANode},
			topo:              twoNUMANodesTopo(),
			coreIDToNodeIDMap: map[int]int{}, // CPU 0 not mapped -> error path
		}
		podRes := []*podresourcesapi.PodResources{
			{
				Namespace: "ns",
				Name:      "pod",
				Containers: []*podresourcesapi.ContainerResources{
					{Name: "c", CpuIds: []int64{0}},
				},
			},
		}
		got, err := rm.encodeContainerAffinities(podRes)
		assert.NoError(t, err)
		want := numaplacement.Payload{
			Containers:     0,
			NUMANodes:      2,
			BusiestNode:    0,
			VectorEncoding: numaplacement.VectorEncodingLEB89,
			Vectors:        map[int]string{},
		}
		assert.Empty(t, cmp.Diff(want, got))
	})

	t.Run("single_numa_topology_short_circuits_without_encoding_containers", func(t *testing.T) {
		rm := &resourceMonitor{
			args:              Args{TopologyManagerPolicy: TMPolicySingleNUMANode},
			topo:              &ghwtopology.Info{Nodes: []*ghwtopology.Node{{ID: 0}}},
			coreIDToNodeIDMap: map[int]int{0: 0},
		}
		podRes := []*podresourcesapi.PodResources{
			{
				Namespace: "ns1",
				Name:      "pod1",
				Containers: []*podresourcesapi.ContainerResources{
					{Name: "cnt1", CpuIds: []int64{0}},
				},
			},
		}
		got, err := rm.encodeContainerAffinities(podRes)
		assert.NoError(t, err)
		// encodeContainerAffinities returns enc.Result() immediately when len(topo.Nodes)==1,
		// without iterating pod resources (no per-container placement on a single NUMA node).
		want := numaplacement.Payload{
			Containers:     0,
			NUMANodes:      1,
			BusiestNode:    0,
			VectorEncoding: numaplacement.VectorEncodingLEB89,
			Vectors:        map[int]string{},
		}
		assert.Empty(t, cmp.Diff(want, got))
	})

	t.Run("encodes_cpu_affinity", func(t *testing.T) {
		rm := &resourceMonitor{
			args:              Args{TopologyManagerPolicy: TMPolicySingleNUMANode},
			topo:              twoNUMANodesTopo(),
			coreIDToNodeIDMap: map[int]int{0: 1},
		}
		podRes := []*podresourcesapi.PodResources{
			{
				Namespace: "ns1",
				Name:      "pod1",
				Containers: []*podresourcesapi.ContainerResources{
					{Name: "cnt1", CpuIds: []int64{0}},
				},
			},
		}
		got, err := rm.encodeContainerAffinities(podRes)
		assert.NoError(t, err)
		want := numaplacement.Payload{
			Containers:     1,
			NUMANodes:      2,
			BusiestNode:    1,
			VectorEncoding: numaplacement.VectorEncodingLEB89,
			Vectors:        map[int]string{},
		}
		assert.Empty(t, cmp.Diff(want, got))
	})

	t.Run("encodes_device_topology_when_no_cpus", func(t *testing.T) {
		rm := &resourceMonitor{
			args:              Args{TopologyManagerPolicy: TMPolicySingleNUMANode},
			topo:              twoNUMANodesTopo(),
			coreIDToNodeIDMap: map[int]int{},
		}
		podRes := []*podresourcesapi.PodResources{
			{
				Namespace: "ns1",
				Name:      "pod1",
				Containers: []*podresourcesapi.ContainerResources{
					{
						Name: "cnt1",
						Devices: []*podresourcesapi.ContainerDevices{
							{
								ResourceName: "fake.io/gpu",
								DeviceIds:    []string{"gpu0"},
								Topology: &podresourcesapi.TopologyInfo{
									Nodes: []*podresourcesapi.NUMANode{{ID: 0}},
								},
							},
						},
					},
				},
			},
		}
		got, err := rm.encodeContainerAffinities(podRes)
		assert.NoError(t, err)
		want := numaplacement.Payload{
			Containers:     1,
			NUMANodes:      2,
			BusiestNode:    0,
			VectorEncoding: numaplacement.VectorEncodingLEB89,
			Vectors:        map[int]string{},
		}
		assert.Empty(t, cmp.Diff(want, got))
	})

	t.Run("encodes_memory_topology_when_no_cpus_or_devices", func(t *testing.T) {
		rm := &resourceMonitor{
			args:              Args{TopologyManagerPolicy: TMPolicySingleNUMANode},
			topo:              twoNUMANodesTopo(),
			coreIDToNodeIDMap: map[int]int{},
		}
		podRes := []*podresourcesapi.PodResources{
			{
				Namespace: "ns1",
				Name:      "pod1",
				Containers: []*podresourcesapi.ContainerResources{
					{
						Name: "cnt1",
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
				},
			},
		}
		got, err := rm.encodeContainerAffinities(podRes)
		assert.NoError(t, err)
		want := numaplacement.Payload{
			Containers:     1,
			NUMANodes:      2,
			BusiestNode:    1,
			VectorEncoding: numaplacement.VectorEncodingLEB89,
			Vectors:        map[int]string{},
		}
		assert.Empty(t, cmp.Diff(want, got))
	})

	t.Run("multiple_pods_and_mixed_skip_encode", func(t *testing.T) {
		rm := &resourceMonitor{
			args:              Args{TopologyManagerPolicy: TMPolicySingleNUMANode},
			topo:              twoNUMANodesTopo(),
			coreIDToNodeIDMap: map[int]int{0: 0, 1: 1},
		}
		podRes := []*podresourcesapi.PodResources{
			{
				Namespace: "ns1",
				Name:      "pod-a",
				Containers: []*podresourcesapi.ContainerResources{
					{Name: "skip-me"},
					{Name: "ok", CpuIds: []int64{0}},
				},
			},
			{
				Namespace: "ns2",
				Name:      "pod-b",
				Containers: []*podresourcesapi.ContainerResources{
					{Name: "ok2", CpuIds: []int64{1}},
				},
			},
		}
		got, err := rm.encodeContainerAffinities(podRes)
		assert.NoError(t, err)
		want := numaplacement.Payload{
			Containers:     2,
			NUMANodes:      2,
			BusiestNode:    0,
			VectorEncoding: numaplacement.VectorEncodingLEB89,
			Vectors:        map[int]string{1: "!"},
		}
		assert.Empty(t, cmp.Diff(want, got))
	})

	t.Run("encode_container_failure_numa_oob_is_silent_no_state_change", func(t *testing.T) {
		rm := &resourceMonitor{
			args:              Args{TopologyManagerPolicy: TMPolicySingleNUMANode},
			topo:              twoNUMANodesTopo(),
			coreIDToNodeIDMap: map[int]int{},
		}
		podRes := []*podresourcesapi.PodResources{
			{
				Namespace: "ns1",
				Name:      "pod1",
				Containers: []*podresourcesapi.ContainerResources{
					{
						Name: "cnt1",
						Devices: []*podresourcesapi.ContainerDevices{
							{
								ResourceName: "fake.io/gpu",
								DeviceIds:    []string{"gpu0"},
								Topology: &podresourcesapi.TopologyInfo{
									Nodes: []*podresourcesapi.NUMANode{{ID: 3}},
								},
							},
						},
					},
				},
			},
		}
		got, err := rm.encodeContainerAffinities(podRes)
		assert.NoError(t, err)
		want := numaplacement.Payload{
			Containers:     0,
			NUMANodes:      2,
			BusiestNode:    0,
			VectorEncoding: numaplacement.VectorEncodingLEB89,
			Vectors:        map[int]string{},
		}
		assert.Empty(t, cmp.Diff(want, got))
	})
}

func scanAttributeValue(attrs topologyv1alpha2.AttributeList, name string) (string, bool) {
	for i := range attrs {
		if attrs[i].Name == name {
			return attrs[i].Value, true
		}
	}
	return "", false
}

func zoneByName(zones topologyv1alpha2.ZoneList, name string) (topologyv1alpha2.Zone, bool) {
	for i := range zones {
		if zones[i].Name == name {
			return zones[i], true
		}
	}
	return topologyv1alpha2.Zone{}, false
}

// TestScan_NodeLevelAndPerZoneAttributes checks ScanResponse.Attributes (node-level NRT attributes):
// pod fingerprint + method, numaplacement metadata (PackMetadata), and per-zone numaplacement vectors.
func TestScan_NodeLevelAndPerZoneAttributes(t *testing.T) {
	fakeTopo := ghwtopology.Info{}
	err := json.Unmarshal([]byte(testTopology), &fakeTopo)
	assert.NoError(t, err)

	allContainerDevices := []*v1.ContainerDevices{
		{
			ResourceName: "fake.io/net",
			DeviceIds:    []string{"netAAA-0"},
			Topology:     &v1.TopologyInfo{Nodes: []*v1.NUMANode{{ID: 0}}},
		},
		{
			ResourceName: "fake.io/net",
			DeviceIds:    []string{"netAAA-1"},
			Topology:     &v1.TopologyInfo{Nodes: []*v1.NUMANode{{ID: 0}}},
		},
		{
			ResourceName: "fake.io/net",
			DeviceIds:    []string{"netAAA-2"},
			Topology:     &v1.TopologyInfo{Nodes: []*v1.NUMANode{{ID: 0}}},
		},
		{
			ResourceName: "fake.io/net",
			DeviceIds:    []string{"netAAA-3"},
			Topology:     &v1.TopologyInfo{Nodes: []*v1.NUMANode{{ID: 0}}},
		},
		{
			ResourceName: "fake.io/net",
			DeviceIds:    []string{"netBBB-0"},
			Topology:     &v1.TopologyInfo{Nodes: []*v1.NUMANode{{ID: 1}}},
		},
		{
			ResourceName: "fake.io/net",
			DeviceIds:    []string{"netBBB-1"},
			Topology:     &v1.TopologyInfo{Nodes: []*v1.NUMANode{{ID: 1}}},
		},
		{
			ResourceName: "fake.io/gpu",
			DeviceIds:    []string{"gpuAAA"},
			Topology:     &v1.TopologyInfo{Nodes: []*v1.NUMANode{{ID: 1}}},
		},
	}

	availRes := &v1.AllocatableResourcesResponse{
		Devices: allContainerDevices,
		CpuIds: []int64{
			0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11,
			12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23,
		},
	}

	mockPodResClient := new(podres.MockPodResourcesListerClient)
	mockPodResClient.On("GetAllocatableResources", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("*v1.AllocatableResourcesRequest")).Return(availRes, nil)

	args := Args{
		PodSetFingerprint:       true,
		PodSetFingerprintMethod: podfingerprint.MethodAll,
		TopologyManagerPolicy:   TMPolicySingleNUMANode,
	}
	resMon, err := NewResourceMonitor(
		Handle{PodResCli: mockPodResClient},
		args,
		WithNodeName("TEST"),
		WithTopology(&fakeTopo),
		WithK8sClient(fake.NewSimpleClientset()),
	)
	assert.NoError(t, err)

	listResp := &v1.ListPodResourcesResponse{
		PodResources: []*v1.PodResources{
			{
				Namespace: "ns1",
				Name:      "pod-a",
				Containers: []*v1.ContainerResources{
					{Name: "ok", CpuIds: []int64{0}},
				},
			},
			{
				Namespace: "ns2",
				Name:      "pod-b",
				Containers: []*v1.ContainerResources{
					{Name: "ok2", CpuIds: []int64{1}},
				},
			},
		},
	}
	mockPodResClient.On("List", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("*v1.ListPodResourcesRequest")).Return(listResp, nil)

	scanRes, err := resMon.Scan(ResourceExclude{})
	assert.NoError(t, err)

	// Node-level (ScanResponse.Attributes): pod fingerprint + method
	fpVal, ok := scanAttributeValue(scanRes.Attributes, podfingerprint.Attribute)
	assert.True(t, ok, "expected %q on scan attributes", podfingerprint.Attribute)
	assert.True(t, strings.HasPrefix(fpVal, podfingerprint.Prefix+podfingerprint.Version),
		"fingerprint value should start with prefix+version, got %q", fpVal)

	methodVal, ok := scanAttributeValue(scanRes.Attributes, podfingerprint.AttributeMethod)
	assert.True(t, ok, "expected %q on scan attributes", podfingerprint.AttributeMethod)
	assert.Equal(t, podfingerprint.MethodAll, methodVal)

	assert.Equal(t, fpVal, scanRes.Annotations[podfingerprint.Annotation])

	metaVal, ok := scanAttributeValue(scanRes.Attributes, numaplacement.AttributeMetadata)
	assert.True(t, ok, "expected %q on scan attributes", numaplacement.AttributeMetadata)
	var unpacked numaplacement.Payload
	assert.NoError(t, numaplacement.UnpackMetadataInto(&unpacked, metaVal))
	assert.Equal(t, 2, unpacked.Containers)
	assert.Equal(t, 2, unpacked.NUMANodes)
	assert.Equal(t, numaplacement.VectorEncodingLEB89, unpacked.VectorEncoding)
	assert.Equal(t, 0, unpacked.BusiestNode)
	// UnpackMetadataInto only fills metadata fields (cc/nn/bn/ve); it does not populate Payload.Vectors.

	// Per-zone: numaplacement vector only on non-busiest NUMA (here NUMA 1); busiest omits wire vector
	zones := scanRes.SortedZones()
	z0, ok := zoneByName(zones, "node-0")
	assert.True(t, ok)
	_, hasVec0 := scanAttributeValue(z0.Attributes, numaplacement.AttributeVector)
	assert.False(t, hasVec0, "busiest NUMA zone should not expose %q", numaplacement.AttributeVector)

	z1, ok := zoneByName(zones, "node-1")
	assert.True(t, ok)
	vec1, ok := scanAttributeValue(z1.Attributes, numaplacement.AttributeVector)
	assert.True(t, ok, "non-busiest NUMA should expose %q", numaplacement.AttributeVector)
	wantVec := numaplacement.Prefix + numaplacement.Version + "!"
	assert.Equal(t, wantVec, vec1, "vector wire form should match numaplacement for this pod/container set")

	// Verify the LEB89 vector payload (same as numaplacement.Payload.Vectors[1] on the encoder side):
	// strip npv0v001 prefix from the zone attribute value, then decode to sorted container indices.
	vecWire := strings.TrimPrefix(vec1, numaplacement.Prefix+numaplacement.Version)
	assert.Equal(t, []int32{0}, numaplacement.DecodePerNUMAVector(vecWire),
		"decoded offsets index into the sorted xxhash order of encoded container IDs")

	mockPodResClient.AssertExpectations(t)
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
