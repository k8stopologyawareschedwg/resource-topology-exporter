package finder

import (
	"log"
)

const (
	hostSysFs = "/host-sys"
)

//ContainerData stores numa resource information of containers
//Assumption: 1 socket per numa node update to a data structure with more granularity

func (c *ContainerData) GetAllocatedCPUs() map[string][]string {
	containerCPUNumaInfo := getContainerCPUNumaInfo(c.ContainerResources.CPUInfo)
	return containerCPUNumaInfo
}

func (c *ContainerData) GetAllocatedDevices() map[string]map[devicePluginResourceName]int {
	containerDeviceNumaInfo := getContainerDeviceNumaInfo(c.ContainerResources.Devices)
	return containerDeviceNumaInfo
}

func getContainerDeviceNumaInfo(devices map[devicePluginResourceName][]*DeviceInfo) map[string]map[devicePluginResourceName]int {
	devicesNumaInfo := map[string]map[devicePluginResourceName]int{}
	for res, devInfo := range devices {
		for _, dev := range devInfo {
			if devicesNumaInfo[dev.NumaNode] == nil {
				count := map[devicePluginResourceName]int{res: 0}
				devicesNumaInfo[dev.NumaNode] = count
			}
			devicesNumaInfo[dev.NumaNode][res]++
		}

	}
	return devicesNumaInfo
}

func getContainerCPUNumaInfo(cpuinfo map[string]string) map[string][]string {
	cpusNumaInfo := map[string][]string{}
	for cpuId, numaNode := range cpuinfo {
		cpusNumaInfo[numaNode] = append(cpusNumaInfo[numaNode], cpuId)
	}
	return cpusNumaInfo
}

type ContainerData struct {
	ContainerName      string
	ContainerResources *Resources
}

type devicePluginResourceName string

type Resources struct {
	CPUInfo map[string]string //cpu to NUMA node
	Devices map[devicePluginResourceName][]*DeviceInfo
}

func NewResources(cpus map[string]string, devs map[devicePluginResourceName][]*DeviceInfo) *Resources {
	return &Resources{
		CPUInfo: cpus,
		Devices: devs,
	}
}

type DeviceInfo struct {
	DeviceId   string
	DeviceFile string
	NumaNode   string
}

func NewDeviceInfo(devId string, devFile string, devNumaNode string) *DeviceInfo {
	log.Printf("NewDeviceInfo devId: %v, devFile: %v, devNumaNode: %v ", devId, devFile, devNumaNode)
	return &DeviceInfo{
		DeviceId:   devId,
		DeviceFile: devFile,
		NumaNode:   devNumaNode,
	}
}

func NewContainerData(name string, res *Resources) *ContainerData {
	log.Printf("Container name: %v numaRes :%v ", name, res)
	return &ContainerData{
		ContainerName:      name,
		ContainerResources: res,
	}

}

func (c *ContainerData) GetContainerResources() *Resources {
	return c.ContainerResources
}
