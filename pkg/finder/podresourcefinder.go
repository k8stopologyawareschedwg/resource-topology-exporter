package finder

import (
	"context"
	"fmt"
	"log"

	"github.com/davecgh/go-spew/spew"

	"k8s.io/api/core/v1"

	podresourcesapi "k8s.io/kubernetes/pkg/kubelet/apis/podresources/v1alpha1"
)

type PodResourceFinder struct {
	args              Args
	podResourceClient podresourcesapi.PodResourcesListerClient
}

func NewPodResourceFinder(args Args, podResourceClient podresourcesapi.PodResourcesListerClient) (Finder, error) {
	finderInstance := &PodResourceFinder{
		args: args,
	}
	if finderInstance.args.Namespace != "" {
		log.Printf("watching namespace %q", finderInstance.args.Namespace)
	} else {
		log.Printf("watching all namespaces")
	}

	finderInstance.podResourceClient = podResourceClient
	return finderInstance, nil
}

func (f *PodResourceFinder) isWatchable(podNamespace string) bool {
	if f.args.Namespace == "" {
		return true
	}
	//TODO:  add an explicit check for guaranteed pods
	return f.args.Namespace == podNamespace
}

func (f *PodResourceFinder) Scan(deviceID2ResourceMap map[string]string) ([]PodResources, error) {
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
			contRes.Resources = append(contRes.Resources, makeDeviceResourceInfo(devs, deviceID2ResourceMap)...)
			log.Printf("pod %q container %q contData=%s\n", podResource.GetName(), container.Name, spew.Sdump(contRes))
			podRes.Containers = append(podRes.Containers, contRes)
		}

		podResData = append(podResData, podRes)

	}

	return podResData, nil
}

func makeCPUResourceInfo(cpus []int64) []ResourceInfo {
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

func makeDeviceResourceInfo(devs map[string][]string, deviceID2ResMap map[string]string) []ResourceInfo {
	var resInfos []ResourceInfo
	//slice of deviceIDs
	deviceIds := make([]string, 0)
	var resName string
	var ok bool
	for _, devIds := range devs {
		for _, devId := range devIds {
			resName, ok = deviceID2ResMap[devId]
			if !ok {
				continue
			}
			deviceIds = append(deviceIds, devId)
		}

	}

	resInfos = append(resInfos, ResourceInfo{
		Name: v1.ResourceName(resName),
		Data: deviceIds,
	})
	return resInfos
}
