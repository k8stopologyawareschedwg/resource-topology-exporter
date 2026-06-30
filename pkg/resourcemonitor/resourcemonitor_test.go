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
	"context"
	"encoding/json"
	"log"
	"sort"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/fake"
	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"
	v1 "k8s.io/kubelet/pkg/apis/podresources/v1"

	"github.com/go-logr/logr"

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
		res := NormalizeContainerDevices(logr.Discard(), availRes.GetDevices(), availRes.GetMemory(), availRes.GetCpuIds(), coreIDToNodeIDMap)
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

func getAllContainerDevices() []*v1.ContainerDevices {
	return []*v1.ContainerDevices{
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
}
func TestResourcesScan(t *testing.T) {
	fakeTopo := ghwtopology.Info{}
	Convey("When recovering test topology from JSON data", t, func() {
		err := json.Unmarshal([]byte(testTopology), &fakeTopo)
		So(err, ShouldBeNil)
	})

	allContainerDevices := getAllContainerDevices()

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
		resMon := NewResourceMonitor(Handle{PodResCli: mockPodResClient}, Args{}, "", WithNodeName("TEST"), WithTopology(&fakeTopo), WithK8sClient(fake.NewSimpleClientset()))
		err := resMon.Setup(context.TODO())
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
			scanRes, err := resMon.Scan(context.TODO(), ResourceExclude{}) // no pods allocation
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
		resMon := NewResourceMonitor(Handle{PodResCli: mockPodResClient}, Args{}, "", WithNodeName("TEST"), WithTopology(&fakeTopo), WithK8sClient(fake.NewSimpleClientset()))
		err := resMon.Setup(context.TODO())
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
			scanRes, err := resMon.Scan(context.TODO(), ResourceExclude{}) // no pods allocation
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
		resMon := NewResourceMonitor(Handle{PodResCli: mockPodResClient}, Args{}, "", WithNodeName("TEST"), WithTopology(&fakeTopo), WithK8sClient(fake.NewSimpleClientset()))
		err := resMon.Setup(context.TODO())
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
			scanRes, err := resMon.Scan(context.TODO(), excludeList)
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
		resMon := NewResourceMonitor(Handle{PodResCli: mockPodResClient}, Args{}, "", WithNodeName("TEST"), WithTopology(&fakeTopo), WithK8sClient(fake.NewSimpleClientset()))
		err := resMon.Setup(context.TODO())
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
			scanRes, err := resMon.Scan(context.TODO(), excludeList)
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
		resMon := NewResourceMonitor(Handle{PodResCli: mockPodResClient}, Args{RefreshNodeResources: true}, "", WithNodeName("TEST"), WithTopology(&fakeTopo), WithK8sClient(fake.NewSimpleClientset()))
		err := resMon.Setup(context.TODO())
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
			scanRes, err := resMon.Scan(context.TODO(), excludeList)
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
		resMon := NewResourceMonitor(Handle{PodResCli: mockPodResClient}, Args{PodSetFingerprint: true}, "", WithNodeName("TEST"), WithTopology(&fakeTopo), WithK8sClient(fake.NewSimpleClientset()))
		err := resMon.Setup(context.TODO())
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
			scanRes, err := resMon.Scan(context.TODO(), ResourceExclude{})

			expectedFP := "pfp0v001fe53c4dbd2c5f4a0" // pre-computed and validated manually
			fp, ok := scanRes.Annotations[podfingerprint.Annotation]
			So(ok, ShouldBeTrue)
			log.Printf("FP %q expected %q", fp, expectedFP)
			So(cmp.Equal(fp, expectedFP), ShouldBeTrue)

			So(err, ShouldBeNil)
		})
	})
}

func TestNewResourceMonitorTMConfig(t *testing.T) {
	var topo ghwtopology.Info
	assert.NoError(t, json.Unmarshal([]byte(testTopology), &topo))

	mockPodResClient := new(podres.MockPodResourcesListerClient)
	mockPodResClient.On("GetAllocatableResources", mock.Anything, mock.Anything).
		Return(&v1.AllocatableResourcesResponse{}, nil)

	tmPolicy := "single-numa-node"
	rm := NewResourceMonitor(
		Handle{PodResCli: mockPodResClient},
		Args{},
		tmPolicy,
		WithNodeName("TEST"),
		WithTopology(&topo),
		WithK8sClient(fake.NewSimpleClientset()),
	)
	assert.NoError(t, rm.Setup(context.TODO()))
	assert.True(t, rm.HasTopologyManagerPolicy(tmPolicy))
}

func TestScanNUMAPlacementAttributes(t *testing.T) {
	baselineRes := &v1.AllocatableResourcesResponse{
		Devices: getAllContainerDevices(),
		// CPUId 0 and 1 are missing from the list below to simulate
		// that they are not allocatable CPUs (kube-reserved or system-reserved)
		CpuIds: []int64{
			2, 3, 4, 5, 6, 7, 8, 9, 10, 11,
			12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23,
		},
	}

	fakeTopo := ghwtopology.Info{}
	Convey("When recovering test topology from JSON data", t, func() {
		err := json.Unmarshal([]byte(testTopology), &fakeTopo)
		So(err, ShouldBeNil)
	})

	newPodResMock := func(listResp *podresourcesapi.ListPodResourcesResponse) *podres.MockPodResourcesListerClient {
		mockPodResClient := new(podres.MockPodResourcesListerClient)
		mockPodResClient.On("GetAllocatableResources", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("*v1.AllocatableResourcesRequest")).Return(baselineRes, nil).Once()
		if listResp != nil {
			mockPodResClient.On("List", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("*v1.ListPodResourcesRequest")).Return(listResp, nil).Once()
		}
		return mockPodResClient
	}

	Convey("When containers are eligible for NUMA placement and are encoded successfully", t, func() {
		listResp := &podresourcesapi.ListPodResourcesResponse{
			// mimics the pod resources that are reported by the kubelet for single-numa-node topology
			// and it's valid to have multiple containers in a pod with the different NUMA node affinity
			// simulating container topology scope
			PodResources: []*podresourcesapi.PodResources{
				// Guaranteed-like: exclusive CPUs.
				// eligible for numaplacement container encoding
				{
					Namespace: "guaranteed-ns",
					Name:      "pod1",
					Containers: []*podresourcesapi.ContainerResources{
						{
							Name:   "app",
							CpuIds: []int64{2},
							Memory: []*podresourcesapi.ContainerMemory{
								{
									MemoryType: "memory",
									Size:       1024,
									Topology: &podresourcesapi.TopologyInfo{
										Nodes: []*podresourcesapi.NUMANode{{ID: 0}},
									},
								},
							},
						},
						{
							Name:   "app-2",
							CpuIds: []int64{4},
							Memory: []*podresourcesapi.ContainerMemory{
								{
									MemoryType: "memory",
									Size:       1024,
									Topology: &podresourcesapi.TopologyInfo{
										Nodes: []*podresourcesapi.NUMANode{{ID: 0}},
									},
								},
							},
						},
					},
				},
				// Burstable/BestEffort-like: multiple containers; only sidecar has a NUMA-local device and are eligible for numaplacement container encoding.
				{
					Namespace: "burst-ns",
					Name:      "pod2",
					Containers: []*podresourcesapi.ContainerResources{
						{Name: "app-shim"},
						{
							Name: "sidecar",
							Devices: []*podresourcesapi.ContainerDevices{
								{
									ResourceName: "fake.io/gpu",
									DeviceIds:    []string{"gpuAAA"},
									Topology: &podresourcesapi.TopologyInfo{
										Nodes: []*podresourcesapi.NUMANode{{ID: 1}},
									},
								},
							},
						},
					},
				},
				// BestEffort/Burstable-like without non-native resources
				// not eligible for numaplacement container encoding
				{
					Namespace: "be-ns",
					Name:      "pod3",
					Containers: []*podresourcesapi.ContainerResources{
						{Name: "c1"},
						{Name: "c2"},
					},
				},
				// Guaranteed-like with NUMA-local memory (no integer CPUs) - eligible for numaplacement container encoding
				{
					Namespace: "mem-ns",
					Name:      "pod4",
					Containers: []*podresourcesapi.ContainerResources{
						{
							Name: "cnt-mem-1",
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
						{
							Name: "cnt-mem-2",
							Memory: []*podresourcesapi.ContainerMemory{
								{
									MemoryType: "memory",
									Size:       1024,
									Topology: &podresourcesapi.TopologyInfo{
										Nodes: []*podresourcesapi.NUMANode{{ID: 0}},
									},
								},
							},
						},
					},
				},
			},
		}
		mockPodResClient := newPodResMock(listResp)

		resMon := NewResourceMonitor(
			Handle{PodResCli: mockPodResClient},
			Args{
				PodSetFingerprint:       true,
				PodSetFingerprintMethod: podfingerprint.MethodAll, // to ensure the containers are filtered also for this
			},
			TopologyManagerPolicySingleNUMANode,
			WithNodeName("TEST"),
			WithTopology(&fakeTopo),
			WithK8sClient(fake.NewSimpleClientset()))
		err := resMon.Setup(context.TODO())
		So(err, ShouldBeNil)

		scanRes, err := resMon.Scan(context.TODO(), ResourceExclude{})
		So(err, ShouldBeNil)

		metaVal, ok := scanAttributeValue(scanRes.Attributes, numaplacement.AttributeMetadata)

		assert.True(t, ok, "expected %q from encoded container NUMA affinities", numaplacement.AttributeMetadata)
		assert.True(t, strings.HasPrefix(metaVal, numaplacement.Prefix+numaplacement.Version), "expected LEB89 prefix but got "+metaVal)
		assert.Contains(t, metaVal, "cc=5")
		assert.Contains(t, metaVal, "nn=2")
		assert.Contains(t, metaVal, "bn=0")

		// NUMA 1 has 2 containers at sorted-hash indices 0 and 2; LEB89 delta-encodes as "!#".
		// **precomputed**
		expectedVectorForNUMA1 := "!#"

		// check zones attributes
		for _, zone := range scanRes.Zones {
			if zone.Name == "node-0" {
				_, ok := scanAttributeValue(zone.Attributes, numaplacement.AttributeVector)
				assert.False(t, ok) // because it's the busiest NUMA
			}
			if zone.Name == "node-1" {
				vectorVal, ok := scanAttributeValue(zone.Attributes, numaplacement.AttributeVector)
				assert.True(t, ok)
				assert.Equal(t, expectedVectorForNUMA1, vectorVal)
			}
		}
		mockPodResClient.AssertExpectations(t)
	})

	Convey("When TM config is not supported for NUMA placement, the NUMA placement metadata is not set", t, func() {
		// TM config is not supported for NUMA placement; metadata value is reported as unsupported.
		listResp := &podresourcesapi.ListPodResourcesResponse{
			PodResources: []*podresourcesapi.PodResources{
				{
					Namespace: "ns",
					Name:      "pod",
					Containers: []*podresourcesapi.ContainerResources{
						{Name: "app", CpuIds: []int64{2}},
					},
				},
			},
		}
		mockPodResClient := newPodResMock(listResp)

		rm := NewResourceMonitor(
			Handle{PodResCli: mockPodResClient},
			Args{
				PodSetFingerprint:       true,
				PodSetFingerprintMethod: podfingerprint.MethodAll,
			},
			"none",
			WithNodeName("TEST"),
			WithTopology(&fakeTopo),
			WithK8sClient(fake.NewSimpleClientset()))
		assert.NoError(t, rm.Setup(context.TODO()))

		scanRes, err := rm.Scan(context.TODO(), ResourceExclude{})
		assert.NoError(t, err)

		_, ok := scanAttributeValue(scanRes.Attributes, numaplacement.AttributeMetadata)
		assert.False(t, ok)

		mockPodResClient.AssertExpectations(t)
	})

	Convey("When encoding containers NUMA affinities fails, the NUMA placement metadata is not set", t, func() {
		// CpuIds [999] is not in MakeCoreIDToNodeIDMap(testTopology) -> NUMA placement collection is cancelled.
		listResp := &podresourcesapi.ListPodResourcesResponse{
			PodResources: []*podresourcesapi.PodResources{
				{
					Namespace: "ns",
					Name:      "pod",
					Containers: []*podresourcesapi.ContainerResources{
						{Name: "bad", CpuIds: []int64{999}},
					},
				},
			},
		}
		mockPodResClient := newPodResMock(listResp)

		rm := NewResourceMonitor(
			Handle{PodResCli: mockPodResClient},
			Args{
				PodSetFingerprint:       true,
				PodSetFingerprintMethod: podfingerprint.MethodAll,
			},
			TopologyManagerPolicySingleNUMANode,
			WithNodeName("TEST"),
			WithTopology(&fakeTopo),
			WithK8sClient(fake.NewSimpleClientset()))
		assert.NoError(t, rm.Setup(context.TODO()))

		scanRes, err := rm.Scan(context.TODO(), ResourceExclude{})
		assert.NoError(t, err)

		_, ok := scanAttributeValue(scanRes.Attributes, numaplacement.AttributeMetadata)
		assert.False(t, ok)

		mockPodResClient.AssertExpectations(t)
	})

	Convey("When PFP is disabled, no NUMA placement metadata is added", t, func() {
		mockPodResClient := newPodResMock(&podresourcesapi.ListPodResourcesResponse{})
		rm := NewResourceMonitor(
			Handle{PodResCli: mockPodResClient},
			Args{PodSetFingerprint: false},
			TopologyManagerPolicySingleNUMANode,
			WithNodeName("TEST"),
			WithTopology(&fakeTopo),
			WithK8sClient(fake.NewSimpleClientset()))
		assert.NoError(t, rm.Setup(context.TODO()))

		scanRes, err := rm.Scan(context.TODO(), ResourceExclude{})
		assert.NoError(t, err)

		_, ok := scanAttributeValue(scanRes.Attributes, numaplacement.AttributeMetadata)
		assert.False(t, ok)

		mockPodResClient.AssertExpectations(t)
	})
}

func TestComputeNUMAPlacementPayload(t *testing.T) {
	Convey("When the topology manager policy is not a single-numa-node (not supported for numaplacement encoding)", t, func() {
		payload := ComputeNUMAPlacementPayload(logr.Discard(), []*podresourcesapi.PodResources{
			{Containers: []*podresourcesapi.ContainerResources{
				{CpuIds: []int64{0}},
			}},
		}, "None", 2, getExpectedCoreToNodeMap())
		assert.Nil(t, payload)
	})

	Convey("When the topology manager policy is single-numa-node", t, func() {
		Convey("With zero pods", func() {
			numaCount := 2
			payload := ComputeNUMAPlacementPayload(logr.Discard(), []*podresourcesapi.PodResources{}, TopologyManagerPolicySingleNUMANode, numaCount, getExpectedCoreToNodeMap())
			assert.NotNil(t, payload)
			assert.Equal(t, 0, payload.Containers)
			assert.Equal(t, 0, payload.BusiestNode)
		})

		Convey("With valid numa-eligible pods", func() {
			pods := []*podresourcesapi.PodResources{
				{
					//has exclusive CPUs
					Name:      "tp1",
					Namespace: "default",
					Containers: []*podresourcesapi.ContainerResources{
						{Name: "cnt1", CpuIds: []int64{1}},
					},
				},
				{
					//burstable/besteffort-like:
					Name:      "tp2",
					Namespace: "default",
					Containers: []*podresourcesapi.ContainerResources{
						{ //numa-eligible container, reason:device
							Name: "cnt2",
							Devices: []*podresourcesapi.ContainerDevices{
								{ResourceName: "fake.io/gpu", DeviceIds: []string{"gpuAAA"}, Topology: &podresourcesapi.TopologyInfo{Nodes: []*podresourcesapi.NUMANode{{ID: 1}}}},
							},
						},
						{
							//non-numa-eligible container
							Name:   "cnt3",
							CpuIds: []int64{},
						},
					},
				},
			}
			numaCount := 2
			payload := ComputeNUMAPlacementPayload(logr.Discard(), pods, TopologyManagerPolicySingleNUMANode, numaCount, getExpectedCoreToNodeMap())
			assert.NotNil(t, payload)
			assert.Equal(t, 2, payload.Containers)
			assert.Equal(t, 1, payload.BusiestNode)
		})

	})

}

func scanAttributeValue(attrs []topologyv1alpha2.AttributeInfo, name string) (string, bool) {
	for i := range attrs {
		if attrs[i].Name == name {
			return attrs[i].Value, true
		}
	}
	return "", false
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

func TestCollectNonTopologyResourceNames(t *testing.T) {
	logger := logr.Discard()

	testCases := []struct {
		name     string
		devices  []*podresourcesapi.ContainerDevices
		expected sets.Set[corev1.ResourceName]
	}{
		{
			name:     "empty devices",
			devices:  []*podresourcesapi.ContainerDevices{},
			expected: sets.New[corev1.ResourceName](),
		},
		{
			name: "devices with NUMA topology are excluded",
			devices: []*podresourcesapi.ContainerDevices{
				{
					ResourceName: "fake.io/net",
					DeviceIds:    []string{"net-0"},
					Topology: &podresourcesapi.TopologyInfo{
						Nodes: []*podresourcesapi.NUMANode{{ID: 0}},
					},
				},
			},
			expected: sets.New[corev1.ResourceName](),
		},
		{
			name: "devices with nil topology are captured",
			devices: []*podresourcesapi.ContainerDevices{
				{
					ResourceName: "fake.io/notopo",
					DeviceIds:    []string{"dev-0"},
					Topology:     nil,
				},
			},
			expected: sets.New[corev1.ResourceName]("fake.io/notopo"),
		},
		{
			name: "devices with empty nodes are captured",
			devices: []*podresourcesapi.ContainerDevices{
				{
					ResourceName: "fake.io/emptynodes",
					DeviceIds:    []string{"dev-0"},
					Topology: &podresourcesapi.TopologyInfo{
						Nodes: []*podresourcesapi.NUMANode{},
					},
				},
			},
			expected: sets.New[corev1.ResourceName]("fake.io/emptynodes"),
		},
		{
			name: "devices with ID -1 are captured",
			devices: []*podresourcesapi.ContainerDevices{
				{
					ResourceName: "fake.io/dontcare",
					DeviceIds:    []string{"dev-0"},
					Topology: &podresourcesapi.TopologyInfo{
						Nodes: []*podresourcesapi.NUMANode{{ID: -1}},
					},
				},
			},
			expected: sets.New[corev1.ResourceName]("fake.io/dontcare"),
		},
		{
			name: "native resources are excluded",
			devices: []*podresourcesapi.ContainerDevices{
				{
					ResourceName: "cpu",
					DeviceIds:    []string{"0"},
					Topology:     nil,
				},
				{
					ResourceName: "memory",
					DeviceIds:    []string{"1024"},
					Topology:     nil,
				},
			},
			expected: sets.New[corev1.ResourceName](),
		},
		{
			name: "mixed devices with and without topology",
			devices: []*podresourcesapi.ContainerDevices{
				{
					ResourceName: "fake.io/with-topo",
					DeviceIds:    []string{"dev-0"},
					Topology: &podresourcesapi.TopologyInfo{
						Nodes: []*podresourcesapi.NUMANode{{ID: 0}},
					},
				},
				{
					ResourceName: "fake.io/no-topo",
					DeviceIds:    []string{"dev-1"},
					Topology:     nil,
				},
				{
					ResourceName: "fake.io/also-no-topo",
					DeviceIds:    []string{"dev-2"},
					Topology: &podresourcesapi.TopologyInfo{
						Nodes: []*podresourcesapi.NUMANode{{ID: -1}},
					},
				},
			},
			expected: sets.New[corev1.ResourceName]("fake.io/no-topo", "fake.io/also-no-topo"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := collectNonTopologyResourceNames(logger, tc.devices)
			assert.True(t, tc.expected.Equal(result), "expected %v, got %v", tc.expected, result)
		})
	}
}

func TestDetectNonDevicePluginResources(t *testing.T) {
	logger := logr.Discard()

	testCases := []struct {
		name                 string
		nodeAllocatable      perNUMAResourceCounter
		nonTopologyResources sets.Set[corev1.ResourceName]
		nodeStatus           corev1.ResourceList
		expected             sets.Set[corev1.ResourceName]
	}{
		{
			name: "detects resources not in device plugins",
			nodeAllocatable: perNUMAResourceCounter{
				0: resourceCounter{"fake.io/net": 4},
			},
			nonTopologyResources: sets.New[corev1.ResourceName](),
			nodeStatus: corev1.ResourceList{
				"cpu":                                    resource.MustParse("24"),
				"memory":                                 resource.MustParse("64Gi"),
				"fake.io/net":                            resource.MustParse("4"),
				"management.workload.openshift.io/cores": resource.MustParse("4000"),
				"another.example.com/resource-without-dp": resource.MustParse("10"),
			},
			expected: sets.New[corev1.ResourceName](
				"management.workload.openshift.io/cores",
				"another.example.com/resource-without-dp",
			),
		},
		{
			name: "does not duplicate already known non-topology resources",
			nodeAllocatable: perNUMAResourceCounter{
				0: resourceCounter{"fake.io/net": 4},
			},
			nonTopologyResources: sets.New[corev1.ResourceName]("fake.io/notopo"),
			nodeStatus: corev1.ResourceList{
				"cpu":            resource.MustParse("24"),
				"fake.io/net":    resource.MustParse("4"),
				"fake.io/notopo": resource.MustParse("2"),
			},
			expected: sets.New[corev1.ResourceName]("fake.io/notopo"),
		},
		{
			name:                 "no extended resources on node",
			nodeAllocatable:      perNUMAResourceCounter{},
			nonTopologyResources: sets.New[corev1.ResourceName](),
			nodeStatus: corev1.ResourceList{
				"cpu":    resource.MustParse("24"),
				"memory": resource.MustParse("64Gi"),
			},
			expected: sets.New[corev1.ResourceName](),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rm := &resourceMonitor{
				nodeAllocatable:      tc.nodeAllocatable,
				nonTopologyResources: tc.nonTopologyResources,
			}
			node := &corev1.Node{
				Status: corev1.NodeStatus{
					Allocatable: tc.nodeStatus,
				},
			}
			rm.detectNonDevicePluginResources(logger, node)
			assert.True(t, tc.expected.Equal(rm.nonTopologyResources),
				"expected %v, got %v", tc.expected, rm.nonTopologyResources)
		})
	}
}

func TestScanNonTopologyResourcesAttribute(t *testing.T) {
	fakeTopo := ghwtopology.Info{}
	assert.NoError(t, json.Unmarshal([]byte(testTopology), &fakeTopo))

	devicesWithTopo := getAllContainerDevices()
	devicesWithNoTopo := append(devicesWithTopo,
		&v1.ContainerDevices{
			ResourceName: "fake.io/notopo-a",
			DeviceIds:    []string{"dev-0", "dev-1"},
			Topology:     nil,
		},
		&v1.ContainerDevices{
			ResourceName: "fake.io/notopo-b",
			DeviceIds:    []string{"dev-x"},
			Topology:     nil,
		},
	)

	availRes := &v1.AllocatableResourcesResponse{
		Devices: devicesWithNoTopo,
		CpuIds: []int64{
			0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11,
			12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23,
		},
	}

	mockPodResClient := new(podres.MockPodResourcesListerClient)
	mockPodResClient.On("GetAllocatableResources", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("*v1.AllocatableResourcesRequest")).Return(availRes, nil)
	resMon := NewResourceMonitor(
		Handle{PodResCli: mockPodResClient},
		Args{},
		"",
		WithNodeName("TEST"),
		WithTopology(&fakeTopo),
		WithK8sClient(fake.NewSimpleClientset()),
	)
	err := resMon.Setup(context.TODO())
	assert.NoError(t, err)

	resp := &v1.ListPodResourcesResponse{
		PodResources: []*v1.PodResources{},
	}
	mockPodResClient.On("List", mock.AnythingOfType("*context.timerCtx"), mock.AnythingOfType("*v1.ListPodResourcesRequest")).Return(resp, nil)

	scanRes, err := resMon.Scan(context.TODO(), ResourceExclude{})
	assert.NoError(t, err)

	var foundAttr *topologyv1alpha2.AttributeInfo
	for i := range scanRes.Attributes {
		if scanRes.Attributes[i].Name == AttributeNodeNonTopologyResources {
			foundAttr = &scanRes.Attributes[i]
			break
		}
	}
	assert.NotNil(t, foundAttr, "nodeNonTopologyResources attribute should be present")

	resNames := strings.Split(foundAttr.Value, ",")
	sort.Strings(resNames)
	assert.Equal(t, []string{"fake.io/notopo-a", "fake.io/notopo-b"}, resNames)
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
