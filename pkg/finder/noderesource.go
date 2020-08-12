package finder

import (
	"fmt"
	"log"
	"strconv"

	"github.com/fromanirh/numalign/pkg/topologyinfo/cpus"
	"github.com/fromanirh/numalign/pkg/topologyinfo/pcidev"
	v1alpha1 "github.com/swatisehgal/topologyapi/pkg/apis/topology/v1alpha1"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

type NodeResources struct {
	// we may want to move to cadvisor past PoC stage
	pciDevs         *pcidev.PCIDevices
	cpus            *cpus.CPUs
	cpuID2NUMAID    map[int]int
	pciAddr2NUMAID  map[string]int
	perNUMACapacity map[int]map[v1.ResourceName]int64
	// pciaddr -> resourcename
	pci2ResourceMap map[string]string
}

func NewNodeResources(sysfs string, pciResMapConf map[string]string) (*NodeResources, error) {

	nodeResourceInstance := &NodeResources{
		perNUMACapacity: make(map[int]map[v1.ResourceName]int64),
	}

	var err error
	// first scan the sysfs
	// CAUTION: these resources are expected to change rarely - if ever. So we are intentionally do this once during the process lifecycle.
	nodeResourceInstance.cpus, err = cpus.NewCPUs(sysfs)
	if err != nil {
		return nil, fmt.Errorf("error scanning the system CPUs: %v", err)
	}
	for nodeNum, cpuList := range nodeResourceInstance.cpus.NUMANodeCPUs {
		log.Printf("detected system CPU: NUMA cell %d cpus = %v\n", nodeNum, cpuList)
	}

	for nodeNum := 0; nodeNum < nodeResourceInstance.cpus.NUMANodes; nodeNum++ {
	}

	nodeResourceInstance.pciDevs, err = pcidev.NewPCIDevices(sysfs)
	if err != nil {
		return nil, fmt.Errorf("error scanning the system PCI devices: %v", err)
	}
	for _, pciDev := range nodeResourceInstance.pciDevs.Items {
		log.Printf("detected system PCI device = %s\n", pciDev.String())
	}

	// helper maps
	var pciDevMap map[int]map[v1.ResourceName]int64
	pciDevMap, nodeResourceInstance.pci2ResourceMap, nodeResourceInstance.pciAddr2NUMAID = makePCI2ResourceMap(nodeResourceInstance.cpus.NUMANodes, nodeResourceInstance.pciDevs, pciResMapConf)
	nodeResourceInstance.cpuID2NUMAID = make(map[int]int)
	for nodeNum, cpus := range nodeResourceInstance.cpus.NUMANodeCPUs {
		for _, cpu := range cpus {
			nodeResourceInstance.cpuID2NUMAID[cpu] = nodeNum
		}
	}

	// initialize with the capacities
	for nodeNum := 0; nodeNum < nodeResourceInstance.cpus.NUMANodes; nodeNum++ {
		nodeResourceInstance.perNUMACapacity[nodeNum] = make(map[v1.ResourceName]int64)
		for resName, count := range pciDevMap[nodeNum] {
			nodeResourceInstance.perNUMACapacity[nodeNum][resName] = count
		}

		cpus := nodeResourceInstance.cpus.NUMANodeCPUs[nodeNum]
		nodeResourceInstance.perNUMACapacity[nodeNum][v1.ResourceCPU] = int64(len(cpus))
	}

	return nodeResourceInstance, nil

}

func (n *NodeResources) GetPCI2ResourceMap() map[string]string {
	return n.pci2ResourceMap
}

func updateNUMAMap(numaData map[int]map[v1.ResourceName]int64, ri ResourceInfo, nodeResourceData *NodeResources) {
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
			numaData[nodeNum][ri.Name]--
		}
		return
	}
	for _, pciAddr := range ri.Data {
		nodeNum, ok := nodeResourceData.pciAddr2NUMAID[pciAddr]
		if !ok {
			log.Printf("unknown PCI address: %q", pciAddr)
			continue
		}
		numaData[nodeNum][ri.Name]--
	}
}

func Aggregate(podResData []PodResources, nodeResourceData *NodeResources) []v1alpha1.NUMANodeResource {
	var perNumaRes []v1alpha1.NUMANodeResource

	perNuma := make(map[int]map[v1.ResourceName]int64)
	for nodeNum, nodeRes := range nodeResourceData.perNUMACapacity {
		perNuma[nodeNum] = make(map[v1.ResourceName]int64)
		for resName, resCap := range nodeRes {
			perNuma[nodeNum][resName] = resCap
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
		numaRes := v1alpha1.NUMANodeResource{
			NUMAID:    nodeNum,
			Resources: make(v1.ResourceList),
		}
		for name, intQty := range resList {
			numaRes.Resources[name] = *resource.NewQuantity(intQty, resource.DecimalSI)
		}
		perNumaRes = append(perNumaRes, numaRes)
	}
	return perNumaRes
}

func makePCI2ResourceMap(numaNodes int, pciDevs *pcidev.PCIDevices, pciResMapConf map[string]string) (map[int]map[v1.ResourceName]int64, map[string]string, map[string]int) {
	pciAddr2NUMAID := make(map[string]int)
	pci2Res := make(map[string]string)

	perNUMACapacity := make(map[int]map[v1.ResourceName]int64)
	for nodeNum := 0; nodeNum < numaNodes; nodeNum++ {
		perNUMACapacity[nodeNum] = make(map[v1.ResourceName]int64)

		for _, pciDev := range pciDevs.Items {
			if pciDev.NUMANode() != nodeNum {
				continue
			}
			sriovDev, ok := pciDev.(pcidev.SRIOVDeviceInfo)
			if !ok {
				continue
			}

			if !sriovDev.IsVFn {
				continue
			}

			resName, ok := pciResMapConf[sriovDev.ParentFn]
			if !ok {
				continue
			}

			pci2Res[sriovDev.Address()] = resName
			pciAddr2NUMAID[sriovDev.Address()] = nodeNum
			perNUMACapacity[nodeNum][v1.ResourceName(resName)]++
		}
	}
	return perNUMACapacity, pci2Res, pciAddr2NUMAID
}
