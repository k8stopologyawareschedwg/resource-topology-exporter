package finder

import (
	"log"
)

//PodResourceData contains resource information of pods
type PodResourceData struct {
	podSandBoxId string
	podUId       string
	podName      string
	//qos string
	namespace      string
	containersData []ContainerData
}

func NewPodResourceData(podSb string, uid string, name string, ns string, contsData []ContainerData) *PodResourceData {
	log.Printf("Pod name: %v, namespace ns:%v", name, ns)
	return &PodResourceData{
		podSandBoxId:   podSb,
		podUId:         uid,
		podName:        name,
		namespace:      ns,
		containersData: contsData,
	}
}

func (p *PodResourceData) GetContainersData() []ContainerData {
	return p.containersData
}

func (p *PodResourceData) GetAllocatedCPUs() map[string][]string {
	podcpusNumaInfo := map[string][]string{}
	for _, c := range p.containersData {
		containerCPUNumaInfo := c.GetAllocatedCPUs()
		for k, cpuList := range containerCPUNumaInfo {
			for _, cpu := range cpuList {
				podcpusNumaInfo[k] = append(podcpusNumaInfo[k], cpu)
			}
		}
	}
	return podcpusNumaInfo
}
func (p *PodResourceData) GetAllocatedDevices() map[string]map[devicePluginResourceName]int {
	podDevsNumaInfo := map[string]map[devicePluginResourceName]int{}
	for _, c := range p.containersData {
		devices := c.GetAllocatedDevices()
		for numaId, devs := range devices {
			for res, n := range devs {
				if podDevsNumaInfo[numaId] == nil {
					count := map[devicePluginResourceName]int{res: 0}
					podDevsNumaInfo[numaId] = count
				}
				podDevsNumaInfo[numaId][res] += n
			}
		}
	}
	return podDevsNumaInfo
}
