package finder

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/davecgh/go-spew/spew"

	"k8s.io/api/core/v1"

	podresources "k8s.io/kubernetes/pkg/kubelet/apis/podresources"
	podresourcesapi "k8s.io/kubernetes/pkg/kubelet/apis/podresources/v1alpha1"
)

const (
	defaultPodResourcesTimeout = 10 * time.Second
	defaultPodResourcesMaxSize = 1024 * 1024 * 16 // 16 Mb
	// obtained these values from node e2e tests : https://github.com/kubernetes/kubernetes/blob/82baa26905c94398a0d19e1b1ecf54eb8acb6029/test/e2e_node/util.go#L70
)

type PodResourceFinder struct {
	args              Args
	podResourceClient podresourcesapi.PodResourcesListerClient
}

func NewPodResourceFinder(args Args, pciResMapConf map[string]string) (Finder, error) {
	finderInstance := &PodResourceFinder{
		args: args,
	}
	var err error
	finderInstance.podResourceClient, _, err = podresources.GetClient(finderInstance.args.PodResourceSocketPath, defaultPodResourcesTimeout, defaultPodResourcesMaxSize)
	if err != nil {
		return nil, fmt.Errorf("Can't create client: %v", err)
	}
	log.Printf("connected to '%v'!", finderInstance.args.PodResourceSocketPath)
	if finderInstance.args.Namespace != "" {
		log.Printf("watching namespace %q", finderInstance.args.Namespace)
	} else {
		log.Printf("watching all namespaces")
	}

	return finderInstance, nil
}

func (f *PodResourceFinder) isWatchable(podNamespace string) bool {
	if f.args.Namespace == "" {
		return true
	}
	//TODO:  add an explicit check for guaranteed pods
	return f.args.Namespace == podNamespace
}

func (f *PodResourceFinder) Scan(pci2ResourceMap map[string]string) ([]PodResources, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultPodResourcesTimeout)
	defer cancel()

	//Pod Resource API client
	resp, err := f.podResourceClient.List(ctx, &podresourcesapi.ListPodResourcesRequest{})
	if err != nil {
		return nil, fmt.Errorf("Can't receive response: %v.Get(_) = _, %v", f.podResourceClient, err)
	}

	var podResData []PodResources

	for _, podResource := range resp.GetPodResources() {
		if !f.isWatchable(podResource.GetNamespace()) {
			log.Printf("SKIP pod %q\n", podResource.Name)
			continue
		}

		podRes := PodResources{
			Name:      podResource.GetName(),
			Namespace: podResource.GetNamespace(),
		}
		for _, container := range podResource.GetContainers() {
			contRes := ContainerResources{
				Name: container.Name,
			}
			devs := make(map[string][]string)
			for _, device := range container.GetDevices() {
				devs[device.ResourceName] = device.DeviceIds
			}
			cpuList := container.GetCpuIds()

			contRes.Resources = append(contRes.Resources, makeCPUResourceInfo(cpuList)...)
			// assumption here is that deviceIds are guaranteed to be PCI addresses
			contRes.Resources = append(contRes.Resources, makePCIDeviceResourceInfo(devs, pci2ResourceMap)...)
			log.Printf("pod %q container %q contData=%s\n", podResource.GetName(), container.Name, spew.Sdump(contRes))
			podRes.Containers = append(podRes.Containers, contRes)
		}

		podResData = append(podResData, podRes)

	}

	return podResData, nil
}

func makeCPUResourceInfo(cpus []uint32) []ResourceInfo {
	var ret []string
	for _, cpuID := range cpus {
		ret = append(ret, fmt.Sprintf("%d", cpuID))
	}
	return []ResourceInfo{
		{
			Name: v1.ResourceCPU,
			Data: ret,
		},
	}
}

func makePCIDeviceResourceInfo(devs map[string][]string, pci2ResMap map[string]string) []ResourceInfo {
	var resInfos []ResourceInfo
	//slice of deviceIDs (It is assumed that device Ids are PCI addresses)
	pciAddrs := make([]string, 0)
	var resName string
	var ok bool
	for _, devIds := range devs {
		for _, devId := range devIds {
			resName, ok = pci2ResMap[devId]
			if !ok {
				continue
			}
			pciAddrs = append(pciAddrs, devId)
		}

	}
	resInfos = append(resInfos, ResourceInfo{
		Name: v1.ResourceName(resName),
		Data: pciAddrs,
	})
	return resInfos
}
