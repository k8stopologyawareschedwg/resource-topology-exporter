/*
Copyright 2026 The Kubernetes Authors.

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
	"fmt"
	"strings"
	"testing"

	"k8s.io/client-go/kubernetes/fake"
	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"

	ghwtopology "github.com/jaypipes/ghw/pkg/topology"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	topologyv1alpha2 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha2"
	"github.com/k8stopologyawareschedwg/numaplacement"
	"github.com/k8stopologyawareschedwg/podfingerprint"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres"
)

func scanAttributeValue(attrs []topologyv1alpha2.AttributeInfo, name string) (string, bool) {
	for i := range attrs {
		if attrs[i].Name == name {
			return attrs[i].Value, true
		}
	}
	return "", false
}

// allocatableDevicesStub matches testTopology NUMA layout (IDs 0 and 1) used across resourcemonitor tests.
func allocatableDevicesStub() []*podresourcesapi.ContainerDevices {
	return []*podresourcesapi.ContainerDevices{
		{ResourceName: "fake.io/net", DeviceIds: []string{"netAAA-0"}, Topology: &podresourcesapi.TopologyInfo{Nodes: []*podresourcesapi.NUMANode{{ID: 0}}}},
		{ResourceName: "fake.io/net", DeviceIds: []string{"netAAA-1"}, Topology: &podresourcesapi.TopologyInfo{Nodes: []*podresourcesapi.NUMANode{{ID: 0}}}},
		{ResourceName: "fake.io/net", DeviceIds: []string{"netAAA-2"}, Topology: &podresourcesapi.TopologyInfo{Nodes: []*podresourcesapi.NUMANode{{ID: 0}}}},
		{ResourceName: "fake.io/net", DeviceIds: []string{"netAAA-3"}, Topology: &podresourcesapi.TopologyInfo{Nodes: []*podresourcesapi.NUMANode{{ID: 0}}}},
		{ResourceName: "fake.io/net", DeviceIds: []string{"netBBB-0"}, Topology: &podresourcesapi.TopologyInfo{Nodes: []*podresourcesapi.NUMANode{{ID: 1}}}},
		{ResourceName: "fake.io/net", DeviceIds: []string{"netBBB-1"}, Topology: &podresourcesapi.TopologyInfo{Nodes: []*podresourcesapi.NUMANode{{ID: 1}}}},
		{ResourceName: "fake.io/gpu", DeviceIds: []string{"gpuAAA"}, Topology: &podresourcesapi.TopologyInfo{Nodes: []*podresourcesapi.NUMANode{{ID: 1}}}},
	}
}

func TestScan_NumaplacementMetadataWhenContainerAffinityEncoded(t *testing.T) {
	fakeTopo := ghwtopology.Info{}
	assert.NoError(t, json.Unmarshal([]byte(testTopology), &fakeTopo))

	mockCli := new(podres.MockPodResourcesListerClient)
	mockCli.On("GetAllocatableResources", mock.Anything, mock.Anything).Return(
		&podresourcesapi.AllocatableResourcesResponse{
			Devices: allocatableDevicesStub(),
			CpuIds:  []int64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23},
		}, nil)

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
	mockCli.On("List", mock.Anything, mock.Anything).Return(listResp, nil)

	rm, err := NewResourceMonitor(Handle{PodResCli: mockCli}, Args{
		PodSetFingerprint:       true,
		PodSetFingerprintMethod: podfingerprint.MethodAll,
		TopologyManagerPolicy:   TopologyManagerPolicySingleNUMANode,
	}, WithNodeName("TEST"), WithTopology(&fakeTopo), WithK8sClient(fake.NewSimpleClientset()))
	assert.NoError(t, err)

	// Build container affinities which belong to NUMA node 0 from listResp.
	expectedEligibleContainers := []numaplacement.ContainerAffinity{
		{
			ID: numaplacement.ContainerID{
				Namespace:     "guaranteed-ns",
				PodName:       "pod1",
				ContainerName: "app",
			},
			NUMANode: 0,
		},
		{
			ID: numaplacement.ContainerID{
				Namespace:     "guaranteed-ns",
				PodName:       "pod1",
				ContainerName: "app-2",
			},
			NUMANode: 0,
		},
		{
			ID: numaplacement.ContainerID{
				Namespace:     "burst-ns",
				PodName:       "pod2",
				ContainerName: "sidecar",
			},
			NUMANode: 1,
		},
		{
			ID: numaplacement.ContainerID{
				Namespace:     "mem-ns",
				PodName:       "pod4",
				ContainerName: "cnt-mem-1",
			},
			NUMANode: 1,
		},
		{
			ID: numaplacement.ContainerID{
				Namespace:     "mem-ns",
				PodName:       "pod4",
				ContainerName: "cnt-mem-2",
			},
			NUMANode: 0,
		},
	}
	eligibleContainers, err := GetEligibleContainersForNUMAPlacement(listResp.PodResources, rm.coreIDToNodeIDMap)
	assert.NoError(t, err)
	assert.Equal(t, expectedEligibleContainers, eligibleContainers)

	expectedPayload, err := rm.computeNUMAPlacementPayload(listResp.PodResources)
	assert.NoError(t, err)

	expectedVectorForNUMA1 := expectedPayload.Vectors[1] // Vectors is a map of numa ID -> vector
	fmt.Println("expectedVectorForNUMA1", expectedVectorForNUMA1)
	scanRes, err := rm.Scan(ResourceExclude{})
	assert.NoError(t, err)

	metaVal, ok := scanAttributeValue(scanRes.Attributes, numaplacement.AttributeMetadata)
	assert.True(t, ok, "expected %q from encoded container NUMA affinities", numaplacement.AttributeMetadata)
	assert.True(t, strings.HasPrefix(metaVal, numaplacement.Prefix+numaplacement.Version))
	assert.Contains(t, metaVal, "cc=5")
	assert.Contains(t, metaVal, "nn=2")
	assert.Contains(t, metaVal, "bn=0")

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
	mockCli.AssertExpectations(t)
}

func TestScan_NumaplacementMetadataEmptyWhenEncodeFails(t *testing.T) {
	fakeTopo := ghwtopology.Info{}
	assert.NoError(t, json.Unmarshal([]byte(testTopology), &fakeTopo))

	mockCli := new(podres.MockPodResourcesListerClient)
	mockCli.On("GetAllocatableResources", mock.Anything, mock.Anything).Return(
		&podresourcesapi.AllocatableResourcesResponse{
			Devices: allocatableDevicesStub(),
			CpuIds:  []int64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23},
		}, nil)

	// CpuIds [999] is not in MakeCoreIDToNodeIDMap(testTopology) -> encodeContainerAffinities errors; metadata value stays empty.
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
	mockCli.On("List", mock.Anything, mock.Anything).Return(listResp, nil)

	rm, err := NewResourceMonitor(Handle{PodResCli: mockCli}, Args{
		PodSetFingerprint:       true,
		PodSetFingerprintMethod: podfingerprint.MethodAll,
		TopologyManagerPolicy:   TopologyManagerPolicySingleNUMANode,
	}, WithNodeName("TEST"), WithTopology(&fakeTopo), WithK8sClient(fake.NewSimpleClientset()))
	assert.NoError(t, err)

	scanRes, err := rm.Scan(ResourceExclude{})
	assert.NoError(t, err)

	metaVal, ok := scanAttributeValue(scanRes.Attributes, numaplacement.AttributeMetadata)
	assert.True(t, ok)
	assert.Equal(t, "", metaVal)

	mockCli.AssertExpectations(t)
}

func TestScan_NoNumaplacementMetadataWithoutPodSetFingerprint(t *testing.T) {
	fakeTopo := ghwtopology.Info{}
	assert.NoError(t, json.Unmarshal([]byte(testTopology), &fakeTopo))

	mockCli := new(podres.MockPodResourcesListerClient)
	mockCli.On("GetAllocatableResources", mock.Anything, mock.Anything).Return(
		&podresourcesapi.AllocatableResourcesResponse{
			Devices: allocatableDevicesStub(),
			CpuIds:  []int64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23},
		}, nil)
	mockCli.On("List", mock.Anything, mock.Anything).Return(
		&podresourcesapi.ListPodResourcesResponse{PodResources: []*podresourcesapi.PodResources{}}, nil)

	rm, err := NewResourceMonitor(Handle{PodResCli: mockCli}, Args{PodSetFingerprint: false}, WithNodeName("TEST"), WithTopology(&fakeTopo), WithK8sClient(fake.NewSimpleClientset()))
	assert.NoError(t, err)

	scanRes, err := rm.Scan(ResourceExclude{})
	assert.NoError(t, err)

	_, ok := scanAttributeValue(scanRes.Attributes, numaplacement.AttributeMetadata)
	assert.False(t, ok)

	mockCli.AssertExpectations(t)
}
