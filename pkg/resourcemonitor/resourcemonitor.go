/*
Copyright 2021 The Kubernetes Authors.

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
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"

	"github.com/jaypipes/ghw"
	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha2"
	topologyv1alpha2 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha2"
	"github.com/k8stopologyawareschedwg/podfingerprint"

	podresfilter "github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres/filter"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres/filter/numalocality"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres/middleware/podexclude"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/prometheus"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/sysinfo"
)

const (
	defaultPodResourcesTimeout = 10 * time.Second
	// obtained these values from node e2e tests : https://github.com/kubernetes/kubernetes/blob/82baa26905c94398a0d19e1b1ecf54eb8acb6029/test/e2e_node/util.go#L70
)

type ResourceExclude map[string][]string

type Args struct {
	Namespace                   string
	SysfsRoot                   string
	ResourceExclude             ResourceExclude
	RefreshNodeResources        bool
	PodSetFingerprint           bool
	PodSetFingerprintMethod     string
	ExposeTiming                bool
	PodSetFingerprintStatusFile string
	PodExclude                  podexclude.List
	ExcludeTerminalPods         bool
}

type Handle struct {
	PodResCli podresourcesapi.PodResourcesListerClient
	K8SCli    kubernetes.Interface
}

type ScanResponse struct {
	Zones       v1alpha2.ZoneList
	Attributes  v1alpha2.AttributeList
	Annotations map[string]string
}

func (sr ScanResponse) SortedZones() v1alpha2.ZoneList {
	res := sr.Zones.DeepCopy()
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
	return res
}

type ResourceMonitor interface {
	Scan(excludeList ResourceExclude) (ScanResponse, error)
}

// ToMapSet keeps the original keys, but replaces values with set.String types
func (rel ResourceExclude) ToMapSet() map[string]sets.String {
	asSet := make(map[string]sets.String)
	for k, v := range rel {
		asSet[k] = sets.NewString(v...)
	}
	return asSet
}

func (rel ResourceExclude) String() string {
	var b strings.Builder
	for name, items := range rel {
		fmt.Fprintf(&b, "- %s: [%s]\n", name, strings.Join(items, ", "))
	}
	return b.String()
}

// mapping resource -> count
type resourceCounter map[v1.ResourceName]int64

// mapping numa cell -> resource counter
type perNUMAResourceCounter map[int]resourceCounter

type resourceMonitor struct {
	nodeName          string
	args              Args
	podResCli         podresourcesapi.PodResourcesListerClient
	k8sCli            kubernetes.Interface
	topo              *ghw.TopologyInfo
	coreIDToNodeIDMap map[int]int
	nodeCapacity      perNUMAResourceCounter
	nodeAllocatable   perNUMAResourceCounter
}

func NewResourceMonitor(hnd Handle, args Args, options ...func(*resourceMonitor)) (*resourceMonitor, error) {
	rm := &resourceMonitor{
		podResCli: hnd.PodResCli,
		k8sCli:    hnd.K8SCli,
		args:      args,
	}
	for _, opt := range options {
		opt(rm)
	}

	if rm.nodeName == "" {
		rm.nodeName = os.Getenv("NODE_NAME")
	}

	klog.Infof("resource monitor for %q starting", rm.nodeName)

	if rm.topo == nil {
		topo, err := ghw.Topology(ghw.WithPathOverrides(ghw.PathOverrides{
			"/sys": args.SysfsRoot,
		}))
		if err != nil {
			return nil, err
		}
		rm.topo = topo
	}

	klog.V(3).Infof("machine topology: %s", toJSON(rm.topo))

	rm.coreIDToNodeIDMap = MakeCoreIDToNodeIDMap(rm.topo)

	if !rm.args.RefreshNodeResources {
		klog.Infof("getting node resources once")
		if err := rm.updateNodeResources(); err != nil {
			return nil, err
		}
	} else {
		klog.Infof("tracking node resources")
		if err := rm.updateNodeResources(); err != nil {
			return nil, err
		}
		if err := addNodeInformerEvent(rm.k8sCli, cache.ResourceEventHandlerFuncs{UpdateFunc: rm.resUpdated}); err != nil {
			return nil, err
		}
	}

	if rm.args.Namespace != "" {
		klog.Infof("watching namespace %q", rm.args.Namespace)
	} else {
		klog.Infof("watching all namespaces")
	}
	return rm, nil
}

func WithTopology(topo *ghw.TopologyInfo) func(*resourceMonitor) {
	return func(rm *resourceMonitor) {
		rm.topo = topo
	}
}

func WithK8sClient(c kubernetes.Interface) func(*resourceMonitor) {
	return func(rm *resourceMonitor) {
		rm.k8sCli = c
	}
}

func WithNodeName(name string) func(*resourceMonitor) {
	return func(rm *resourceMonitor) {
		rm.nodeName = name
	}
}

func (rm *resourceMonitor) Scan(excludeList ResourceExclude) (ScanResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultPodResourcesTimeout)
	defer cancel()
	resp, err := rm.podResCli.List(ctx, &podresourcesapi.ListPodResourcesRequest{})
	if err != nil {
		prometheus.UpdatePodResourceApiCallsFailureMetric("list")
		return ScanResponse{}, err
	}

	st := podfingerprint.MakeStatus(rm.nodeName)
	scanRes := ScanResponse{
		Attributes:  topologyv1alpha2.AttributeList{},
		Annotations: map[string]string{},
	}

	respPodRes := resp.GetPodResources()

	if rm.args.PodSetFingerprint {
		podresFilter := numalocality.Required
		if rm.args.PodSetFingerprintMethod == podfingerprint.MethodAll {
			podresFilter = podresfilter.AlwaysPass
		}
		pfpSign := ComputePodFingerprint(respPodRes, &st, podresFilter)
		scanRes.Attributes = append(scanRes.Attributes, topologyv1alpha2.AttributeInfo{
			Name:  podfingerprint.Attribute,
			Value: pfpSign,
		})
		scanRes.Attributes = append(scanRes.Attributes, topologyv1alpha2.AttributeInfo{
			Name:  podfingerprint.AttributeMethod,
			Value: rm.args.PodSetFingerprintMethod,
		})
		scanRes.Annotations[podfingerprint.Annotation] = pfpSign
		klog.V(6).Infof("pfp: " + st.Repr())
	}

	allDevs := GetAllContainerDevices(respPodRes, rm.args.Namespace, rm.coreIDToNodeIDMap)
	allocated := ContainerDevicesToPerNUMAResourceCounters(allDevs)

	excludeSet := excludeList.ToMapSet()
	zones := make(topologyv1alpha2.ZoneList, 0, len(rm.topo.Nodes))
	// if there are no allocatable resources under a NUMA we might ended up with holes in the NRT objects.
	// this is why we're using the topology info and not the nodeAllocatable
	for nodeID := range rm.topo.Nodes {
		zone := topologyv1alpha2.Zone{
			Name:      makeZoneName(nodeID),
			Type:      "Node",
			Resources: make(topologyv1alpha2.ResourceInfoList, 0),
		}

		costs, err := makeCostsPerNumaNode(rm.topo.Nodes, nodeID)
		if err != nil {
			klog.Warningf("cannot find costs for NUMA node %d: %v", nodeID, err)
		} else {
			zone.Costs = costs
		}

		resCapCounters, ok := rm.nodeCapacity[nodeID]
		if !ok {
			resCapCounters = make(resourceCounter)
		}
		// the case of zero-value is handled below

		// check if NUMA has some allocatable resources
		resCounters, ok := rm.nodeAllocatable[nodeID]
		if !ok {
			// NUMA node doesn't have any allocatable resources. This means the returned counters map is empty.
			// Yet, the node exists in the topology, thus we consider all its CPUs are reserved
			resCounters = make(resourceCounter)
			resCounters[v1.ResourceCPU] = 0
		}

		for resName, resAlloc := range resCounters {
			if inExcludeSet(excludeSet, resName, rm.nodeName) {
				continue
			}

			resCapacity, ok := resCapCounters[resName]
			if !ok || resCapacity == 0 {
				resCapacity = resAlloc

				// some legitimate (e.g. not obsolete) device plugin may not report the topology, hence we will
				// be in this block. Let's differentiate the severity: we should never get here for core resources,
				// while we can for devices. There's not a simple/comfortable way to detect buggy plugins, so
				// in case of non-native resources, let's tolerate and let's log only when very high levels are requested.
				// In these cases the admin knows there could be A LOT of data in the logs.
				if isNativeResource(resName) {
					klog.Warningf("zero capacity for native resource %q on NUMA cell %d", resName, nodeID)
				} else {
					klog.V(5).Infof("zero capacity for extra resource %q on NUMA cell %d", resName, nodeID)
				}
			}
			if resAlloc > resCapacity {
				klog.Warningf("allocated more than capacity for %q on zone %q", resName.String(), zone.Name)
				// we trust more kubelet than ourselves atm.
				resCapacity = resAlloc
			}

			resUsed := allocated[nodeID][resName]

			resAvail := resAlloc - resUsed
			if resAvail < 0 {
				klog.Warningf("negative size for %q on zone %q", resName.String(), zone.Name)
				resAvail = 0
			}

			zone.Resources = append(zone.Resources, topologyv1alpha2.ResourceInfo{
				Name:        resName.String(),
				Available:   *resource.NewQuantity(resAvail, resource.DecimalSI),
				Allocatable: *resource.NewQuantity(resAlloc, resource.DecimalSI),
				Capacity:    *resource.NewQuantity(resCapacity, resource.DecimalSI),
			})
		}

		zones = append(zones, zone)
	}
	scanRes.Zones = zones

	if rm.args.PodSetFingerprint && rm.args.PodSetFingerprintStatusFile != "" {
		dir, file := filepath.Split(rm.args.PodSetFingerprintStatusFile)
		err := toFile(st, dir, file)
		klog.V(6).InfoS("error dumping the pfp status", "fullPath", rm.args.PodSetFingerprintStatusFile, "statusFile", file, "err", err)
		// intentionally ignore error, we must keep going.
	}
	return scanRes, nil
}

func (rm *resourceMonitor) updateNodeCapacity() error {
	memCounters, err := sysinfo.GetMemoryResourceCounters(sysinfo.Handle{})
	if err != nil {
		return err
	}

	hp2Mi := sysinfo.HugepageResourceNameFromSize(sysinfo.HugepageSize2Mi)
	hp1Gi := sysinfo.HugepageResourceNameFromSize(sysinfo.HugepageSize1Gi)

	// we care only about reservable resources, thus:
	// cpu, memory, hugepages
	perNUMARc := make(perNUMAResourceCounter)
	for nodeID := range rm.topo.Nodes {
		perNUMARc[nodeID] = resourceCounter{
			v1.ResourceCPU:         cpuCapacity(rm.topo, nodeID),
			v1.ResourceMemory:      memCounters[string(v1.ResourceMemory)][nodeID],
			v1.ResourceName(hp2Mi): memCounters[hp2Mi][nodeID],
			v1.ResourceName(hp1Gi): memCounters[hp1Gi][nodeID],
		}
	}
	rm.nodeCapacity = perNUMARc
	return nil
}

func (rm *resourceMonitor) resUpdated(old, new interface{}) {
	nOld := old.(*v1.Node)
	nNew := new.(*v1.Node)

	if nNew.Name != rm.nodeName {
		return
	}

	// the status frequency update are configurable via the node-status-update-frequency option in Kubelet
	if !reflect.DeepEqual(nOld.Status.Capacity, nNew.Status.Capacity) ||
		!reflect.DeepEqual(nOld.Status.Allocatable, nNew.Status.Allocatable) {
		klog.V(2).Infof("update node resources")
		if err := rm.updateNodeResources(); err != nil {
			klog.ErrorS(err, "while updating node resources")
		}
	}
}

func (rm *resourceMonitor) updateNodeAllocatable() error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultPodResourcesTimeout)
	defer cancel()
	allocRes, err := rm.podResCli.GetAllocatableResources(ctx, &podresourcesapi.AllocatableResourcesRequest{})
	if err != nil {
		prometheus.UpdatePodResourceApiCallsFailureMetric("get_allocatable_resources")
		return err
	}

	allDevs := NormalizeContainerDevices(allocRes.GetDevices(), allocRes.GetMemory(), allocRes.GetCpuIds(), rm.coreIDToNodeIDMap)
	rm.nodeAllocatable = ContainerDevicesToPerNUMAResourceCounters(allDevs)
	return nil
}

func (rm *resourceMonitor) updateDevicesCapacity() {
	for numaId, resourceCnt := range rm.nodeAllocatable {
		for resName, quan := range resourceCnt {
			if isNativeResource(resName) {
				continue
			}
			capacityResCnt := rm.nodeCapacity[numaId]
			capacityResCnt[resName] = quan
		}
	}
}

func (rm *resourceMonitor) updateNodeResources() error {
	if err := rm.updateNodeCapacity(); err != nil {
		return fmt.Errorf("error while updating node capacity: %w", err)
	}
	if err := rm.updateNodeAllocatable(); err != nil {
		return fmt.Errorf("error while updating node allocatable: %w", err)
	}
	// there is no trivial way to detect devices capacity from the node.
	// hence, initialize capacity as allocatable
	rm.updateDevicesCapacity()
	return nil
}

func GetAllContainerDevices(podRes []*podresourcesapi.PodResources, namespace string, coreIDToNodeIDMap map[int]int) []*podresourcesapi.ContainerDevices {
	allCntRes := []*podresourcesapi.ContainerDevices{}
	for _, pr := range podRes {
		// filter by namespace (if given)
		if namespace != "" && namespace != pr.GetNamespace() {
			continue
		}
		for _, cnt := range pr.GetContainers() {
			allCntRes = append(allCntRes, NormalizeContainerDevices(cnt.GetDevices(), cnt.GetMemory(), cnt.GetCpuIds(), coreIDToNodeIDMap)...)
		}

	}
	return allCntRes
}

func ComputePodFingerprint(podRes []*podresourcesapi.PodResources, st *podfingerprint.Status, allowFilter func(*podresourcesapi.PodResources) bool) string {
	fp := podfingerprint.NewTracingFingerprint(len(podRes), st)
	for _, pr := range podRes {
		if !allowFilter(pr) {
			continue
		}
		fp.AddPod(pr)
	}
	return fp.Sign()
}

func NormalizeContainerDevices(devices []*podresourcesapi.ContainerDevices, memoryBlocks []*podresourcesapi.ContainerMemory, cpuIds []int64, coreIDToNodeIDMap map[int]int) []*podresourcesapi.ContainerDevices {
	contDevs := append([]*podresourcesapi.ContainerDevices{}, devices...)

	cpusPerNuma := make(map[int][]string)
	for _, cpuID := range cpuIds {
		nodeID, ok := coreIDToNodeIDMap[int(cpuID)]
		if !ok {
			klog.Warningf("cannot find the NUMA node for CPU %d", cpuID)
			continue
		}
		cpusPerNuma[nodeID] = append(cpusPerNuma[nodeID], fmt.Sprintf("%d", cpuID))
	}

	for nodeID, cpuList := range cpusPerNuma {
		contDevs = append(contDevs, &podresourcesapi.ContainerDevices{
			ResourceName: string(v1.ResourceCPU),
			DeviceIds:    cpuList,
			Topology: &podresourcesapi.TopologyInfo{
				Nodes: []*podresourcesapi.NUMANode{
					{ID: int64(nodeID)},
				},
			},
		})
	}

	for _, block := range memoryBlocks {
		blockSize := block.GetSize_()
		if blockSize == 0 {
			continue
		}

		for _, node := range block.GetTopology().GetNodes() {
			contDevs = append(contDevs, &podresourcesapi.ContainerDevices{
				ResourceName: block.MemoryType,
				DeviceIds:    []string{fmt.Sprintf("%d", blockSize)},
				Topology: &podresourcesapi.TopologyInfo{
					Nodes: []*podresourcesapi.NUMANode{
						{ID: int64(node.ID)},
					},
				},
			})
		}
	}
	return contDevs
}

func ContainerDevicesToPerNUMAResourceCounters(devices []*podresourcesapi.ContainerDevices) perNUMAResourceCounter {
	perNUMARc := make(perNUMAResourceCounter)
	for _, device := range devices {
		resourceName := device.GetResourceName()
		for _, node := range device.GetTopology().GetNodes() {
			nodeID := int(node.GetID())
			nodeRes, ok := perNUMARc[nodeID]
			if !ok {
				nodeRes = make(resourceCounter)
			}
			if resourceName == string(v1.ResourceMemory) || strings.HasPrefix(resourceName, v1.ResourceHugePagesPrefix) {
				var memSize int64
				for _, devBlock := range device.GetDeviceIds() {
					// can't fail, we constructed in a correct way
					devBlockSize, _ := strconv.ParseInt(devBlock, 10, 64)
					memSize += devBlockSize
				}
				nodeRes[v1.ResourceName(resourceName)] += memSize
			} else {
				nodeRes[v1.ResourceName(resourceName)] += int64(len(device.GetDeviceIds()))
			}
			perNUMARc[nodeID] = nodeRes
		}
	}
	return perNUMARc
}

func MakeCoreIDToNodeIDMap(topo *ghw.TopologyInfo) map[int]int {
	coreToNode := make(map[int]int)
	for _, node := range topo.Nodes {
		for _, core := range node.Cores {
			for _, procID := range core.LogicalProcessors {
				coreToNode[procID] = node.ID
			}
		}
	}
	klog.V(5).Infof("CPU mapping: %s", mapIntIntToString(coreToNode))
	return coreToNode
}

// makeCostsPerNumaNode builds the cost map to reach all the known NUMA zones (mapping (numa zone) -> cost) starting from the given NUMA zone.
func makeCostsPerNumaNode(nodes []*ghw.TopologyNode, nodeIDSrc int) ([]topologyv1alpha2.CostInfo, error) {
	nodeSrc := findNodeByID(nodes, nodeIDSrc)
	if nodeSrc == nil {
		return nil, fmt.Errorf("unknown node: %d", nodeIDSrc)
	}
	nodeCosts := make([]topologyv1alpha2.CostInfo, 0, len(nodeSrc.Distances))
	for nodeIDDst, dist := range nodeSrc.Distances {
		// TODO: this assumes there are no holes (= no offline node) in the distance vector
		nodeCosts = append(nodeCosts, topologyv1alpha2.CostInfo{
			Name:  makeZoneName(nodeIDDst),
			Value: int64(dist),
		})
	}
	return nodeCosts, nil
}

// makeZoneName returns the canonical name of a NUMA zone from its ID.
func makeZoneName(nodeID int) string {
	return fmt.Sprintf("node-%d", nodeID)
}

func findNodeByID(nodes []*ghw.TopologyNode, nodeID int) *ghw.TopologyNode {
	for _, node := range nodes {
		if node.ID == nodeID {
			return node
		}
	}
	return nil
}

func inExcludeSet(excludeSet map[string]sets.String, resName v1.ResourceName, nodeName string) bool {
	if set, ok := excludeSet["*"]; ok && set.Has(string(resName)) {
		return true
	}
	if set, ok := excludeSet[nodeName]; ok && set.Has(string(resName)) {
		return true
	}
	return false
}

func cpuCapacity(topo *ghw.TopologyInfo, nodeID int) int64 {
	nodeSrc := findNodeByID(topo.Nodes, nodeID)
	logicalCoresPerNUMA := 0
	for _, core := range nodeSrc.Cores {
		logicalCoresPerNUMA += len(core.LogicalProcessors)
	}
	return int64(logicalCoresPerNUMA)
}

func addNodeInformerEvent(c kubernetes.Interface, handler cache.ResourceEventHandlerFuncs) error {
	factory := informers.NewSharedInformerFactory(c, 0)
	nodeInformer := factory.Core().V1().Nodes().Informer()
	nodeInformer.AddEventHandler(handler)
	ctx := context.Background()
	factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), nodeInformer.HasSynced) {
		return fmt.Errorf("timed out waiting for caches to sync")
	}
	return nil
}

// isNativeResource return true if the given resource is a core kubernetes resource (e.g. not provided by external device plugins)
func isNativeResource(resName v1.ResourceName) bool {
	return resName == v1.ResourceCPU || resName == v1.ResourceMemory || strings.HasPrefix(string(resName), v1.ResourceHugePagesPrefix)
}

func toFile(st podfingerprint.Status, dir, file string) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}

	dst, err := os.CreateTemp(dir, "__"+file)
	if err != nil {
		return err
	}
	defer os.Remove(dst.Name()) // either way, we need to get rid of this

	_, err = dst.Write(data)
	if err != nil {
		return err
	}

	err = dst.Close()
	if err != nil {
		return err
	}

	return os.Rename(dst.Name(), filepath.Join(dir, file))
}

func PFPMethodSupported() string {
	methods := []string{
		podfingerprint.MethodAll,
		podfingerprint.MethodWithExclusiveResources,
	}
	return strings.Join(methods, ",")
}

func PFPMethodIsSupported(value string) (string, error) {
	val := strings.ToLower(value)
	if val == podfingerprint.MethodAll || val == podfingerprint.MethodWithExclusiveResources {
		return val, nil
	}
	return val, fmt.Errorf("unsupported method  %q", value)
}

func mapIntIntToString(mii map[int]int) string {
	var sb strings.Builder
	for key, val := range mii {
		fmt.Fprintf(&sb, "%d:%d ", key, val)
	}
	return sb.String()
}

func toJSON(obj interface{}) string {
	data, err := json.Marshal(obj)
	if err != nil {
		return "<ERROR>"
	}
	return string(data)
}
