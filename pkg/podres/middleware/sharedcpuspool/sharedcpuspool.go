package sharedcpuspool

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"

	"google.golang.org/grpc"

	"k8s.io/klog/v2"
	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/k8simported/cpuset"
)

type ContainerIdent struct {
	Namespace     string
	PodName       string
	ContainerName string
}

func (ci *ContainerIdent) String() string {
	if ci == nil {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s", ci.Namespace, ci.PodName, ci.ContainerName)
}

func (ci *ContainerIdent) IsEmpty() bool {
	return ci.Namespace == "" || ci.PodName == "" || ci.ContainerName == ""
}

func ContainerIdentFromEnv() *ContainerIdent {
	cntIdent := ContainerIdent{
		Namespace:     os.Getenv("REFERENCE_NAMESPACE"),
		PodName:       os.Getenv("REFERENCE_POD_NAME"),
		ContainerName: os.Getenv("REFERENCE_CONTAINER_NAME"),
	}
	if cntIdent.IsEmpty() {
		return nil
	}
	return &cntIdent
}

func ContainerIdentFromString(ident string) (*ContainerIdent, error) {
	if ident == "" {
		return nil, nil
	}
	items := strings.Split(ident, "/")
	if len(items) != 3 {
		return nil, fmt.Errorf("malformed ident: %q", ident)
	}
	cntIdent := &ContainerIdent{
		Namespace:     strings.TrimSpace(items[0]),
		PodName:       strings.TrimSpace(items[1]),
		ContainerName: strings.TrimSpace(items[2]),
	}
	klog.Infof("reference container: %s", cntIdent)
	return cntIdent, nil
}

type PodResourcesFilter interface {
	FilterListResponse(resp *podresourcesapi.ListPodResourcesResponse) *podresourcesapi.ListPodResourcesResponse
	FilterAllocatableResponse(resp *podresourcesapi.AllocatableResourcesResponse) *podresourcesapi.AllocatableResourcesResponse
}

type filteringClient struct {
	debug          bool
	cli            podresourcesapi.PodResourcesListerClient
	refCnt         *ContainerIdent
	sharedPoolCPUs cpuset.CPUSet // used only for logging
}

func (fc *filteringClient) FilterListResponse(resp *podresourcesapi.ListPodResourcesResponse) *podresourcesapi.ListPodResourcesResponse {
	sharedPoolCPUs := findSharedPoolCPUsInListResponse(fc.refCnt, resp.GetPodResources())
	if !fc.sharedPoolCPUs.Equals(sharedPoolCPUs) {
		klog.V(2).Infof("detected shared pool change: %q -> %q", fc.sharedPoolCPUs.String(), sharedPoolCPUs.String())
		fc.sharedPoolCPUs = sharedPoolCPUs
	}
	for _, podRes := range resp.GetPodResources() {
		for _, cntRes := range podRes.GetContainers() {
			cpuIds := removeCPUs(cntRes.CpuIds, sharedPoolCPUs)
			if fc.debug && !reflect.DeepEqual(cpuIds, cntRes.CpuIds) {
				curCpus := newCPUSetInt64(cntRes.CpuIds...)
				newCpus := newCPUSetInt64(cpuIds...)
				klog.Infof("performed pool change for %s/%s: %q -> %q", podRes.Name, cntRes.Name, curCpus.String(), newCpus.String())
			}
			cntRes.CpuIds = cpuIds
		}
	}
	return resp
}

func (fc *filteringClient) FilterAllocatableResponse(resp *podresourcesapi.AllocatableResourcesResponse) *podresourcesapi.AllocatableResourcesResponse {
	return resp // nothing to do here
}

func (fc *filteringClient) FilterGetResponse(resp *podresourcesapi.GetPodResourcesResponse) *podresourcesapi.GetPodResourcesResponse {
	return resp // TODO: not needed, but implement actual filtering for consistency
}

func (fc *filteringClient) List(ctx context.Context, in *podresourcesapi.ListPodResourcesRequest, opts ...grpc.CallOption) (*podresourcesapi.ListPodResourcesResponse, error) {
	resp, err := fc.cli.List(ctx, in, opts...)
	if err != nil {
		return resp, err
	}
	return fc.FilterListResponse(resp), nil
}

func (fc *filteringClient) GetAllocatableResources(ctx context.Context, in *podresourcesapi.AllocatableResourcesRequest, opts ...grpc.CallOption) (*podresourcesapi.AllocatableResourcesResponse, error) {
	resp, err := fc.cli.GetAllocatableResources(ctx, in, opts...)
	if err != nil {
		return resp, err
	}
	return fc.FilterAllocatableResponse(resp), nil
}

func (fc *filteringClient) Get(ctx context.Context, in *podresourcesapi.GetPodResourcesRequest, opts ...grpc.CallOption) (*podresourcesapi.GetPodResourcesResponse, error) {
	resp, err := fc.cli.Get(ctx, in, opts...)
	if err != nil {
		return resp, err
	}
	return fc.FilterGetResponse(resp), nil
}

func NewFromLister(cli podresourcesapi.PodResourcesListerClient, debug bool, referenceContainer *ContainerIdent) podresourcesapi.PodResourcesListerClient {
	return &filteringClient{
		debug:  debug,
		cli:    cli,
		refCnt: referenceContainer,
	}
}

func findSharedPoolCPUsInListResponse(refCnt *ContainerIdent, podResources []*podresourcesapi.PodResources) cpuset.CPUSet {
	if refCnt == nil || podResources == nil {
		return cpuset.CPUSet{}
	}
	for _, podRes := range podResources {
		if podRes.Namespace != refCnt.Namespace {
			continue
		}
		if podRes.Name != refCnt.PodName {
			continue
		}
		for _, cntRes := range podRes.GetContainers() {
			if cntRes.Name != refCnt.ContainerName {
				continue
			}
			return newCPUSetInt64(cntRes.CpuIds...)
		}
	}
	return cpuset.CPUSet{}
}

func removeCPUs(cpuIDs []int64, toRemove cpuset.CPUSet) []int64 {
	cs := newCPUSetInt64(cpuIDs...)
	res := cs.Difference(toRemove)
	cpuList := []int64{}
	for _, cpu := range res.List() {
		cpuList = append(cpuList, int64(cpu))
	}
	return cpuList
}

func newCPUSetInt64(cpus ...int64) cpuset.CPUSet {
	cpuList := []int{}
	for _, cpu := range cpus {
		cpuList = append(cpuList, int(cpu))
	}
	return cpuset.New(cpuList...)
}
