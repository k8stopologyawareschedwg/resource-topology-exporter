package finder

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	v1alpha1 "github.com/swatisehgal/topologyapi/pkg/apis/topology/v1alpha1"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	podresourcesapi "k8s.io/kubernetes/pkg/kubelet/apis/podresources/v1alpha1"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

const (
	defaultPodResourcesTimeout = 10 * time.Second
	// obtained these values from node e2e tests : https://github.com/kubernetes/kubernetes/blob/82baa26905c94398a0d19e1b1ecf54eb8acb6029/test/e2e_node/util.go#L70
	PathDevsSysCPU  = "devices/system/cpu"
	PathDevsSysNode = "devices/system/node"
)

type NodeResources struct {
	devices         []*podresourcesapi.ContainerDevices
	CPUs            []int64
	NUMANode2CPUs   map[int][]int
	cpuID2NUMAID    map[int]int
	deviceId2NUMAID map[string]int
	perNUMACapacity map[int]map[v1.ResourceName]int64
	// deviceID -> resourcename
	deviceID2ResourceMap map[string]string
}

type ResourceData struct {
	allocatable int64
	capacity    int64
}

func NewNodeResources(sysfs string, podResourceClient podresourcesapi.PodResourcesListerClient) (*NodeResources, error) {
	nodeResourceInstance := &NodeResources{
		perNUMACapacity: make(map[int]map[v1.ResourceName]int64),
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultPodResourcesTimeout)
	defer cancel()

	var err error
	//Pod Resource API client
	resp, err := podResourceClient.GetAvailableResources(ctx, &podresourcesapi.AvailableResourcesRequest{})
	if err != nil {
		return nil, fmt.Errorf("Can't receive response: %v.Get(_) = _, %v", podResourceClient, err)
	}
	nodeResourceInstance.devices = resp.GetDevices()

	var numaNodes []int
	var cpu2NUMA map[int]int
	numaNodes, nodeResourceInstance.NUMANode2CPUs, cpu2NUMA, err = getNodeCPUInfo(sysfs)
	if err != nil {
		return nil, fmt.Errorf("Error in obtaining node CPU information: %v", err)
	}
	// This is to ensure that we account for only the cpus obtained from podresource API
	nodeResourceInstance.cpuID2NUMAID = make(map[int]int)
	for _, cpuId := range resp.GetCpuIds() {
		nodeResourceInstance.cpuID2NUMAID[int(cpuId)] = cpu2NUMA[int(cpuId)]
	}

	// helper maps
	var devMap map[int]map[v1.ResourceName]int64
	devMap, nodeResourceInstance.deviceID2ResourceMap, nodeResourceInstance.deviceId2NUMAID = makeDeviceResourceMap(len(numaNodes), nodeResourceInstance.devices)

	// initialize with the capacities
	for nodeNum := 0; nodeNum < len(numaNodes); nodeNum++ {
		nodeResourceInstance.perNUMACapacity[nodeNum] = make(map[v1.ResourceName]int64)
		for resName, count := range devMap[nodeNum] {
			nodeResourceInstance.perNUMACapacity[nodeNum][resName] = count
		}

		cpus := nodeResourceInstance.NUMANode2CPUs[nodeNum]
		nodeResourceInstance.perNUMACapacity[nodeNum][v1.ResourceCPU] = int64(len(cpus))
	}

	return nodeResourceInstance, nil

}
func getNodeCPUInfo(sysfs string) ([]int, map[int][]int, map[int]int, error) {

	// get list of numanodes from sysfs by querying /sys/devices/system/node/online
	numanodes, err := getList(filepath.Join(sysfs, PathDevsSysNode, "online"))
	if err != nil {
		return nil, nil, nil, err
	}

	NUMANode2CPUs := make(map[int][]int)
	for _, node := range numanodes {
		cpus, err := getList(filepath.Join(sysfs, PathDevsSysNode, fmt.Sprintf("node%d", node), "cpulist"))
		if err != nil {
			return nil, nil, nil, err
		}
		NUMANode2CPUs[node] = cpus
	}

	cpu2NUMA := make(map[int]int)
	for nodeNum, cpuList := range NUMANode2CPUs {
		log.Printf("detected system CPU: NUMA cell %d cpus = %v\n", nodeNum, cpuList)
		for _, cpu := range cpuList {
			cpu2NUMA[cpu] = nodeNum
		}
	}
	return numanodes, NUMANode2CPUs, cpu2NUMA, nil
}
func getList(path string) ([]int, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cpus, err := cpuset.Parse(strings.TrimSpace(string(data)))
	if err != nil {
		return nil, err
	}
	return cpus.ToSlice(), nil
}

func (n *NodeResources) GetDeviceResourceMap() map[string]string {
	return n.deviceID2ResourceMap
}

func updateNUMAMap(numaData map[int]map[v1.ResourceName]*ResourceData, ri ResourceInfo, nodeResourceData *NodeResources) {
	if ri.Name == v1.ResourceCPU {
		for _, cpuIDStr := range ri.Data {
			cpuID, err := strconv.Atoi(cpuIDStr)
			if err != nil {
				log.Printf("cannot convert cpuID: %q", cpuIDStr)
				continue
			}
			nodeNum, ok := nodeResourceData.cpuID2NUMAID[cpuID]
			if !ok {
				log.Printf("unknown cpuID: %d", cpuID)
				continue
			}
			numaData[nodeNum][ri.Name].allocatable--
		}
		return
	}
	for _, devId := range ri.Data {
		nodeNum, ok := nodeResourceData.deviceId2NUMAID[devId]
		if !ok {
			log.Printf("unknown device: %q", devId)
			continue
		}
		numaData[nodeNum][ri.Name].allocatable--
	}
}

func Aggregate(podResData []PodResources, nodeResourceData *NodeResources) v1alpha1.ZoneMap {
	zones := make(v1alpha1.ZoneMap)

	perNuma := make(map[int]map[v1.ResourceName]*ResourceData)
	for nodeNum, nodeRes := range nodeResourceData.perNUMACapacity {
		perNuma[nodeNum] = make(map[v1.ResourceName]*ResourceData)
		for resName, resCap := range nodeRes {
			perNuma[nodeNum][resName] = &ResourceData{capacity: resCap, allocatable: resCap}
		}
	}

	for _, podRes := range podResData {
		for _, contRes := range podRes.Containers {
			for _, res := range contRes.Resources {
				updateNUMAMap(perNuma, res, nodeResourceData)
			}
		}
	}

	for nodeNum, resList := range perNuma {
		zoneName := fmt.Sprintf("node-%d", nodeNum)
		zone := v1alpha1.Zone{
			Type:      "Node",
			Resources: make(v1alpha1.ResourceInfoMap),
		}
		for name, resData := range resList {
			allocatableQty := *resource.NewQuantity(resData.allocatable, resource.DecimalSI)
			capacityQty := *resource.NewQuantity(resData.capacity, resource.DecimalSI)
			zone.Resources[name.String()] = v1alpha1.ResourceInfo{Allocatable: allocatableQty.String(), Capacity: capacityQty.String()}
		}
		zones[zoneName] = zone
	}
	return zones
}

func makeDeviceResourceMap(numaNodes int, devices []*podresourcesapi.ContainerDevices) (map[int]map[v1.ResourceName]int64, map[string]string, map[string]int) {
	deviceId2NUMAID := make(map[string]int)
	deviceId2Res := make(map[string]string)

	perNUMACapacity := make(map[int]map[v1.ResourceName]int64)
	for nodeNum := 0; nodeNum < numaNodes; nodeNum++ {
		perNUMACapacity[nodeNum] = make(map[v1.ResourceName]int64)
	}
	for _, device := range devices {
		resourceName := device.GetResourceName()
		var nodeNuma int64
		for _, node := range device.GetTopology().GetNodes() {
			nodeNuma = node.GetID()
		}
		for _, deviceId := range device.GetDeviceIds() {
			deviceId2Res[deviceId] = resourceName
			deviceId2NUMAID[deviceId] = int(nodeNuma)
			perNUMACapacity[int(nodeNuma)][v1.ResourceName(resourceName)]++
		}
	}
	return perNUMACapacity, deviceId2Res, deviceId2NUMAID
}
