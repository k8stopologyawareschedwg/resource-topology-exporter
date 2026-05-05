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
		PodResources: []*podresourcesapi.PodResources{
			{
				Namespace: "ns1",
				Name:      "pod-a",
				Containers: []*podresourcesapi.ContainerResources{
					{Name: "c1", CpuIds: []int64{0}},
				},
			},
			{
				Namespace: "ns2",
				Name:      "pod-b",
				Containers: []*podresourcesapi.ContainerResources{
					{Name: "c2", CpuIds: []int64{1}},
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
	assert.True(t, ok, "expected %q from encoded container NUMA affinities", numaplacement.AttributeMetadata)
	assert.True(t, strings.HasPrefix(metaVal, numaplacement.Prefix+numaplacement.Version))
	assert.Contains(t, metaVal, "cc=")
	assert.Contains(t, metaVal, "nn=")

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
