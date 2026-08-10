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

	"github.com/go-logr/logr"

	ghwoption "github.com/jaypipes/ghw/pkg/option"
	ghwtopology "github.com/jaypipes/ghw/pkg/topology"
	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha2"
	topologyv1alpha2 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha2"
	"github.com/k8stopologyawareschedwg/numaplacement"
	"github.com/k8stopologyawareschedwg/podfingerprint"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/metrics"
	numaloclib "github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/numalocality"
	podresfilter "github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres/filter"
	numalocfilter "github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres/filter/numalocality"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres/middleware/podexclude"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/sysinfo"
)

const (
	defaultPodResourcesTimeout = 10 * time.Second
	// obtained these values from node e2e tests : https://github.com/kubernetes/kubernetes/blob/82baa26905c94398a0d19e1b1ecf54eb8acb6029/test/e2e_node/util.go#L70

	TopologyManagerPolicySingleNUMANode = "single-numa-node"

	NUMAPlacementModeContainer = "container"
	NUMAPlacementModeNone      = "none"
)

type ResourceExclude map[string][]string

func (re ResourceExclude) Clone() map[string][]string {
	ret := make(map[string][]string)
	for key, vals := range re {
		ret[key] = append([]string{}, vals...)
	}
	return ret
}

type Args struct {
	Namespace                   string          `json:"namespace,omitempty"`
	SysfsRoot                   string          `json:"sysfsRoot,omitempty"`
	ResourceExclude             ResourceExclude `json:"resourceExclude,omitempty"`
	RefreshNodeResources        bool            `json:"refreshNodeResources,omitempty"`
	PodSetFingerprint           bool            `json:"podSetFingerprint,omitempty"`
	PodSetFingerprintMethod     string          `json:"podSetFingerprintMethod,omitempty"`
	ExposeTiming                bool            `json:"exposeTiming,omitempty"`
	PodSetFingerprintStatusFile string          `json:"podSetFingerprintStatusFile,omitempty"`
	PodExclude                  podexclude.List `json:"podExclude,omitempty"`
	ExcludeTerminalPods         bool            `json:"excludeTerminalPods,omitempty"`
	NUMAPlacement               string          `json:"numaPlacement,omitempty"`
}

func (args Args) Clone() Args {
	return Args{
		Namespace:                   args.Namespace,
		SysfsRoot:                   args.SysfsRoot,
		ResourceExclude:             args.ResourceExclude.Clone(),
		RefreshNodeResources:        args.RefreshNodeResources,
		PodSetFingerprint:           args.PodSetFingerprint,
		PodSetFingerprintMethod:     args.PodSetFingerprintMethod,
		ExposeTiming:                args.ExposeTiming,
		PodSetFingerprintStatusFile: args.PodSetFingerprintStatusFile,
		PodExclude:                  args.PodExclude.Clone(),
		ExcludeTerminalPods:         args.ExcludeTerminalPods,
		NUMAPlacement:               args.NUMAPlacement,
	}
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
	Setup(ctx context.Context) error
	Scan(ctx context.Context, excludeList ResourceExclude) (ScanResponse, error)
}

// ToMapSet keeps the original keys, but replaces values with set.String types
func (rel ResourceExclude) ToMapSet() map[string]sets.Set[string] {
	asSet := make(map[string]sets.Set[string])
	for k, v := range rel {
		asSet[k] = sets.New[string](v...)
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

func (rc resourceCounter) String() string {
	if len(rc) == 0 {
		return ""
	}
	var sb strings.Builder
	for key, val := range rc {
		fmt.Fprintf(&sb, ", %s=%d", string(key), val)
	}
	return sb.String()[2:] // cuts first stray ", " sep
}

// mapping numa cell -> resource counter
type perNUMAResourceCounter map[int]resourceCounter

func (nrc perNUMAResourceCounter) String() string {
	if len(nrc) == 0 {
		return ""
	}
	var sb strings.Builder
	for key, val := range nrc {
		fmt.Fprintf(&sb, "; %d=<%s>", key, val.String())
	}
	return sb.String()[2:] // cuts first stray "; " sep
}

type resourceMonitor struct {
	nodeName          string
	args              Args
	tmPolicy          string
	podResCli         podresourcesapi.PodResourcesListerClient
	k8sCli            kubernetes.Interface
	topo              *ghwtopology.Info
	coreIDToNodeIDMap map[int]int
	nodeCapacity      perNUMAResourceCounter
	nodeAllocatable   perNUMAResourceCounter
	scanIteration     uint64
}

func NewResourceMonitor(hnd Handle, args Args, tmPolicy string, options ...func(*resourceMonitor)) *resourceMonitor {
	rm := &resourceMonitor{
		podResCli: hnd.PodResCli,
		k8sCli:    hnd.K8SCli,
		tmPolicy:  tmPolicy,
		args:      args,
	}
	for _, opt := range options {
		opt(rm)
	}

	if rm.nodeName == "" {
		rm.nodeName = os.Getenv("NODE_NAME")
	}

	return rm
}

func (rm *resourceMonitor) GetScanIteration() string {
	val := rm.scanIteration
	rm.scanIteration++
	return fmt.Sprintf("%v", val)
}

func (rm *resourceMonitor) Setup(ctx context.Context) error {
	logger := klog.FromContext(ctx).WithValues("node", rm.nodeName)
	logger.Info("starting")

	if rm.topo == nil {
		topo, err := ghwtopology.New(ghwoption.WithPathOverrides(ghwoption.PathOverrides{
			"/sys": rm.args.SysfsRoot,
		}))
		if err != nil {
			return err
		}
		rm.topo = topo
	}
	logger.V(4).Info("machine topology", "topology", toJSON(rm.topo))

	rm.coreIDToNodeIDMap = MakeCoreIDToNodeIDMap(rm.topo)
	logger.V(4).Info("CPU mapping", "coreIDToNodeID", mapIntIntToString(rm.coreIDToNodeIDMap))

	if err := rm.updateNodeResources(ctx); err != nil {
		return err
	}
	logger.V(2).Info("initial capacity", "capacity", rm.nodeCapacity)
	logger.V(2).Info("initial allocatable", "allocatable", rm.nodeAllocatable)

	if !rm.args.RefreshNodeResources {
		logger.Info("getting node resources once")
	} else {
		logger.Info("tracking node resources")
		if err := addNodeInformerEvent(ctx, rm.k8sCli, cache.ResourceEventHandlerFuncs{UpdateFunc: func(old, new any) {
			rm.resUpdated(ctx, old, new)
		}}); err != nil {
			return err
		}
	}

	if rm.args.Namespace != "" {
		logger.Info("watching namespace", "namespace", rm.args.Namespace)
	} else {
		logger.Info("watching all namespaces")
	}
	return nil
}

func WithTopology(topo *ghwtopology.Info) func(*resourceMonitor) {
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

func (rm *resourceMonitor) HasSingleNUMANodeTopologyManagerPolicy() bool {
	return rm.tmPolicy == TopologyManagerPolicySingleNUMANode
}

// Scan scans the node pods using podresources API, processes the scan response and builds up the returned value.
func (rm *resourceMonitor) Scan(ctx context.Context, excludeList ResourceExclude) (ScanResponse, error) {
	logger := klog.FromContext(ctx).WithName("resmon").WithValues("scanID", rm.GetScanIteration())

	ctx, cancel := context.WithTimeout(ctx, defaultPodResourcesTimeout)
	defer cancel()
	resp, err := rm.podResCli.List(ctx, &podresourcesapi.ListPodResourcesRequest{})
	if err != nil {
		metrics.UpdatePodResourceApiCallsFailuresMetric("list")
		return ScanResponse{}, err
	}

	respRawPodRes := resp.GetPodResources()
	logger.V(6).Info("raw podresources list", "podresources", stringifyPodResources(respRawPodRes))

	numaEligiblePodRes := []*podresourcesapi.PodResources{}
	for _, pr := range respRawPodRes {
		nl := numalocfilter.Verify(pr)
		if !nl.Allow {
			logger.V(8).Info("podresources item: not eligible for NUMA", "pod", pr.Namespace+"/"+pr.Name, "ident", nl.Ident, "reason", nl.Reason)
			continue
		}
		numaEligiblePodRes = append(numaEligiblePodRes, pr)
	}
	logger.V(6).Info("NUMA eligible podresources list", "podresources", stringifyPodResources(numaEligiblePodRes))

	scanRes := ScanResponse{
		Attributes:  topologyv1alpha2.AttributeList{},
		Annotations: map[string]string{},
	}

	containerNUMAPlacementEnabled := (rm.args.NUMAPlacement == NUMAPlacementModeContainer)
	var payload numaplacement.Payload
	if rm.args.PodSetFingerprint {
		st := podfingerprint.MakeStatus(rm.nodeName)
		pfpPodResources := numaEligiblePodRes
		if rm.args.PodSetFingerprintMethod == podfingerprint.MethodAll {
			pfpPodResources = respRawPodRes
		}
		pfpSign := computePodFingerprintFromPodResources(pfpPodResources, &st, podresfilter.VerifyAlwaysPass)
		scanRes.Attributes = append(scanRes.Attributes, topologyv1alpha2.AttributeInfo{
			Name:  podfingerprint.Attribute,
			Value: pfpSign,
		})
		scanRes.Attributes = append(scanRes.Attributes, topologyv1alpha2.AttributeInfo{
			Name:  podfingerprint.AttributeMethod,
			Value: rm.args.PodSetFingerprintMethod,
		})
		scanRes.Annotations[podfingerprint.Annotation] = pfpSign
		logger.V(6).Info("pod fingerprint", "status", st.Repr())

		podfingerprint.MarkCompleted(st)

		// numaplacement encoding is only done for pods that are eligible for NUMA placement (with
		// exclusive resources), which are a subset of pods that are participating in PFP (it is
		// not always the same pods as PFP because it depends on the filter function used)
		if containerNUMAPlacementEnabled {
			payload, err = ComputeNUMAPlacementPayload(logger, numaEligiblePodRes, rm.tmPolicy, len(rm.topo.Nodes), rm.coreIDToNodeIDMap)
			logger.V(2).Info("containers NUMA-placement detection", "error", err)
			if err == nil {
				metadata := payload.PackMetadata()
				scanRes.Attributes = append(scanRes.Attributes, topologyv1alpha2.AttributeInfo{
					Name:  numaplacement.AttributeMetadata,
					Value: metadata,
				})
				logger.V(6).Info("numaplacement metadata", "metadata", metadata)
			}
		}
	}
	allDevs := GetAllContainerDevices(logger, respRawPodRes, rm.args.Namespace, rm.coreIDToNodeIDMap)
	allocated := ContainerDevicesToPerNUMAResourceCounters(logger, allDevs)

	excludeSet := excludeList.ToMapSet()
	zones := make(topologyv1alpha2.ZoneList, 0, len(rm.topo.Nodes))
	// if there are no allocatable resources under a NUMA we might end up with holes in the NRT objects.
	// this is why we're using the topology info and not the nodeAllocatable
	for nodeID := range rm.topo.Nodes {
		zone := topologyv1alpha2.Zone{
			Name:      makeZoneName(nodeID),
			Type:      "Node",
			Resources: make(topologyv1alpha2.ResourceInfoList, 0),
		}

		if containerNUMAPlacementEnabled {
			zoneVector, ok := payload.Vectors[nodeID]
			if ok {
				zone.Attributes = append(zone.Attributes, topologyv1alpha2.AttributeInfo{
					Name:  numaplacement.AttributeVector,
					Value: zoneVector,
				})
			}
		}

		costs, err := makeCostsPerNumaNode(rm.topo.Nodes, nodeID)
		if err != nil {
			logger.V(1).Info("cannot find costs for NUMA node", "nodeID", nodeID, "err", err)
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
					logger.V(1).Info("zero capacity for native resource", "resource", resName, "numaCell", nodeID)
				} else {
					logger.V(5).Info("zero capacity for extra resource", "resource", resName, "numaCell", nodeID)
				}
			}
			if resAlloc > resCapacity {
				logger.V(1).Info("allocated more than capacity", "resource", resName, "zone", zone.Name)
				// we trust more kubelet than ourselves atm.
				resCapacity = resAlloc
			}

			resUsed := allocated[nodeID][resName]

			resAvail := resAlloc - resUsed
			if resAvail < 0 {
				logger.V(1).Info("negative size", "resource", resName, "zone", zone.Name)
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
	return scanRes, nil
}

func ComputeNUMAPlacementPayload(logger logr.Logger, numaEligiblePodRes []*podresourcesapi.PodResources, topologyManagerPolicy string, nodesCount int, coreIDToNodeIDMap map[int]int) (numaplacement.Payload, error) {
	// computing payload is valuable only on singleNumaNode policy because only under that
	// condition there will be 1:1 stable mapping between the containers to the NUMA nodes.
	if topologyManagerPolicy != TopologyManagerPolicySingleNUMANode {
		return numaplacement.Payload{}, fmt.Errorf("topology manager policy not supported for NUMA placement: %s", topologyManagerPolicy)
	}

	enc, err := numaplacement.NewEncoder(nodesCount)
	if err != nil {
		return numaplacement.Payload{}, fmt.Errorf("while creating NUMA placement encoder: %w", err)
	}

	// Important:the consumer should apply the same filter on the pods' containers to be able
	// to properly decode the attributes' values.
	logger.V(4).Info("collecting containers that are eligible for NUMA placement")
	for _, pr := range numaEligiblePodRes {
		for _, cnt := range pr.Containers {
			cntID := numaplacement.ContainerID{
				Namespace:     pr.Namespace,
				PodName:       pr.Name,
				ContainerName: cnt.Name,
			}
			numaNodeID, err := getContainerSingleNUMAPlacement(cnt, coreIDToNodeIDMap)
			if err != nil {
				return numaplacement.Payload{}, fmt.Errorf("while getting container NUMA placement: containerID=%s, error=%w", cntID.String(), err)
			}
			if numaNodeID == -1 {
				// it's possible that a container does not belong to specific NUMA node (e.g. using shared pool resources)
				//  in that case we skip it
				continue
			}

			enc, err = enc.Encode(numaplacement.ContainerAffinity{
				ID:       cntID,
				NUMANode: numaNodeID,
			})
			if err != nil {
				return numaplacement.Payload{}, fmt.Errorf("while encoding container NUMA affinity: containerID=%s, error=%w", cntID.String(), err)
			}
			logger.V(6).Info("encoded container NUMA Affinity", "ident", cntID.String(), "numaNode", numaNodeID)
		}
	}

	payload, err := enc.Result()
	if err != nil {
		return numaplacement.Payload{}, fmt.Errorf("while getting NUMA placement encoder result: error=%w", err)
	}

	logger.V(4).Info("encoded NUMA placement payload",
		"containers", payload.Containers,
		"numaNodes", payload.NUMANodes,
		"busiestNode", payload.BusiestNode,
		"encoding", payload.VectorEncoding)
	return payload, nil
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

func (rm *resourceMonitor) resUpdated(ctx context.Context, old, new any) {
	logger := klog.FromContext(ctx)
	nOld := old.(*v1.Node)
	nNew := new.(*v1.Node)

	if nNew.Name != rm.nodeName {
		return
	}

	if !reflect.DeepEqual(nOld.Status.Capacity, nNew.Status.Capacity) ||
		!reflect.DeepEqual(nOld.Status.Allocatable, nNew.Status.Allocatable) {
		logger.V(2).Info("update node resources")
		if err := rm.updateNodeResources(ctx); err != nil {
			logger.Error(err, "while updating node resources")
		}
	}
}

func (rm *resourceMonitor) updateNodeAllocatable(ctx context.Context) error {
	logger := klog.FromContext(ctx)
	ctx, cancel := context.WithTimeout(ctx, defaultPodResourcesTimeout)
	defer cancel()
	allocRes, err := rm.podResCli.GetAllocatableResources(ctx, &podresourcesapi.AllocatableResourcesRequest{})
	if err != nil {
		metrics.UpdatePodResourceApiCallsFailuresMetric("get_allocatable_resources")
		return err
	}

	allDevs := NormalizeContainerDevices(logger, allocRes.GetDevices(), allocRes.GetMemory(), allocRes.GetCpuIds(), rm.coreIDToNodeIDMap)
	rm.nodeAllocatable = ContainerDevicesToPerNUMAResourceCounters(logger, allDevs)
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

func (rm *resourceMonitor) updateNodeResources(ctx context.Context) error {
	if err := rm.updateNodeCapacity(); err != nil {
		return fmt.Errorf("error while updating node capacity: %w", err)
	}
	if err := rm.updateNodeAllocatable(ctx); err != nil {
		return fmt.Errorf("error while updating node allocatable: %w", err)
	}
	// there is no trivial way to detect devices capacity from the node.
	// hence, initialize capacity as allocatable
	rm.updateDevicesCapacity()
	return nil
}

// computePodFingerprintFromPodResources computes the pod fingerprint from the given pod resources after applying the filter function to the pod resources.
// If required that all pods to be included in the fingerprint, pass a no-op verifyFunc that allows all pods.
func computePodFingerprintFromPodResources(podRes []*podresourcesapi.PodResources, st *podfingerprint.Status, verifyFunc func(*podresourcesapi.PodResources) podresfilter.Result) string {
	fp := podfingerprint.NewTracingFingerprint(len(podRes), st)
	for _, pr := range podRes {
		res := verifyFunc(pr)
		if !res.Allow {
			continue
		}
		_ = fp.AddPod(pr)
	}
	return fp.Sign()
}

func stringifyPodResources(podRes []*podresourcesapi.PodResources) string {
	var sb strings.Builder
	sep := ""
	for _, pr := range podRes {
		// note the separator is 2 spaces
		sb.WriteString(sep + pr.Namespace + "/" + pr.Name)
		sep = "  "
	}
	val := sb.String()
	if len(val) == 0 {
		return ""
	}
	return val
}

// GetAllContainerDevices is deprecated and will be unexported in a future version
func GetAllContainerDevices(logger logr.Logger, podRes []*podresourcesapi.PodResources, namespace string, coreIDToNodeIDMap map[int]int) []*podresourcesapi.ContainerDevices {
	allCntRes := []*podresourcesapi.ContainerDevices{}
	for _, pr := range podRes {
		if namespace != "" && namespace != pr.GetNamespace() {
			continue
		}
		for _, cnt := range pr.GetContainers() {
			allCntRes = append(allCntRes, NormalizeContainerDevices(logger, cnt.GetDevices(), cnt.GetMemory(), cnt.GetCpuIds(), coreIDToNodeIDMap)...)
		}
	}
	return allCntRes
}

// ComputePodFingerprint is deprecated and will be unexported in a future version
func ComputePodFingerprint(podRes []*podresourcesapi.PodResources, st *podfingerprint.Status, allowFilter func(*podresourcesapi.PodResources) bool) string {
	fp := podfingerprint.NewTracingFingerprint(len(podRes), st)
	for _, pr := range podRes {
		if !allowFilter(pr) {
			continue
		}
		_ = fp.AddPod(pr)
	}
	return fp.Sign()
}

func NormalizeContainerDevices(logger logr.Logger, devices []*podresourcesapi.ContainerDevices, memoryBlocks []*podresourcesapi.ContainerMemory, cpuIds []int64, coreIDToNodeIDMap map[int]int) []*podresourcesapi.ContainerDevices {
	logger.V(4).Info("normalizing container devices", "devices", len(devices), "memoryBlocks", len(memoryBlocks), "CPUs", len(cpuIds))

	logger.V(4).Info("normalize devices", "count", len(devices))
	contDevs := append([]*podresourcesapi.ContainerDevices{}, devices...)

	cpusPerNuma := make(map[int][]string)
	for _, cpuID := range cpuIds {
		nodeID, ok := coreIDToNodeIDMap[int(cpuID)]
		if !ok {
			logger.V(1).Info("cannot find the NUMA node for CPU", "cpuID", cpuID)
			continue
		}
		cpusPerNuma[nodeID] = append(cpusPerNuma[nodeID], fmt.Sprintf("%d", cpuID))
	}

	for nodeID, cpuList := range cpusPerNuma {
		logger.V(4).Info("normalize CPUs", "numaNode", nodeID, "count", len(cpuList))
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
		blockSize := block.GetSize()
		if blockSize == 0 {
			continue
		}

		for _, node := range block.GetTopology().GetNodes() {
			logger.V(4).Info("normalize memory blocks", "numaNode", node.ID, "size", blockSize)
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

	logger.V(4).Info("normalized container devices", "entries", len(contDevs), "devices", len(devices), "memoryBlocks", len(memoryBlocks), "CPUs", len(cpuIds))
	return contDevs
}

func ContainerDevicesToPerNUMAResourceCounters(logger logr.Logger, devices []*podresourcesapi.ContainerDevices) perNUMAResourceCounter {
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
	logger.V(6).Info("container devices to per-NUMA resource counters", "devices", len(devices), "counters", perNUMARc.String())
	return perNUMARc
}

func MakeCoreIDToNodeIDMap(topo *ghwtopology.Info) map[int]int {
	coreToNode := make(map[int]int)
	for _, node := range topo.Nodes {
		for _, core := range node.Cores {
			for _, procID := range core.LogicalProcessors {
				coreToNode[procID] = node.ID
			}
		}
	}
	return coreToNode
}

// makeCostsPerNumaNode builds the cost map to reach all the known NUMA zones (mapping (numa zone) -> cost) starting from the given NUMA zone.
func makeCostsPerNumaNode(nodes []*ghwtopology.Node, nodeIDSrc int) ([]topologyv1alpha2.CostInfo, error) {
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

func findNodeByID(nodes []*ghwtopology.Node, nodeID int) *ghwtopology.Node {
	for _, node := range nodes {
		if node.ID == nodeID {
			return node
		}
	}
	return nil
}

func inExcludeSet(excludeSet map[string]sets.Set[string], resName v1.ResourceName, nodeName string) bool {
	if set, ok := excludeSet["*"]; ok && set.Has(string(resName)) {
		return true
	}
	if set, ok := excludeSet[nodeName]; ok && set.Has(string(resName)) {
		return true
	}
	return false
}

func cpuCapacity(topo *ghwtopology.Info, nodeID int) int64 {
	nodeSrc := findNodeByID(topo.Nodes, nodeID)
	logicalCoresPerNUMA := 0
	for _, core := range nodeSrc.Cores {
		logicalCoresPerNUMA += len(core.LogicalProcessors)
	}
	return int64(logicalCoresPerNUMA)
}

func addNodeInformerEvent(ctx context.Context, c kubernetes.Interface, handler cache.ResourceEventHandlerFuncs) error {
	factory := informers.NewSharedInformerFactory(c, 0)
	nodeInformer := factory.Core().V1().Nodes().Informer()
	_, _ = nodeInformer.AddEventHandler(handler)
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

// NUMAPlacementModeIsSupported checks that value is a recognised NUMA placement
// reporting mode: NUMAPlacementModeContainer or NUMAPlacementModeNone. Unlike the
// other IsSupported helpers, the empty string is not a valid value: the mode must
// be set explicitly.
func NUMAPlacementModeIsSupported(value string) (string, error) {
	if value == NUMAPlacementModeContainer || value == NUMAPlacementModeNone {
		return value, nil
	}
	return value, fmt.Errorf("unsupported numa placement mode %q: must be %q or %q", value, NUMAPlacementModeContainer, NUMAPlacementModeNone)
}

func mapIntIntToString(mii map[int]int) string {
	var sb strings.Builder
	for key, val := range mii {
		fmt.Fprintf(&sb, "%d:%d ", key, val)
	}
	return sb.String()
}

func toJSON(obj any) string {
	data, err := json.Marshal(obj)
	if err != nil {
		return "<ERROR>"
	}
	return string(data)
}

// getContainerSingleNUMAPlacement returns the NUMA node ID for a container under single-numa-node topology manager policy.
// It relies on kubelet to guarantee that, hence exits the moment it finds the first non (-1) NUMA ID. if no NUMA affinity
// is found, it returns -1.
func getContainerSingleNUMAPlacement(cnt *podresourcesapi.ContainerResources, coreIDToNodeIDMap map[int]int) (int, error) {
	if len(cnt.CpuIds) > 0 {
		// since this is running on singleNUMANode policy, we can trust that all of the CPUs are on the same NUMA node
		nodeID, ok := coreIDToNodeIDMap[int(cnt.CpuIds[0])]
		if !ok {
			//should never happen with singleNUMANode policy, but if it does we should not encode any data as it would be unreliable.
			return -1, fmt.Errorf("CPU ID %d not found in coreIDToNodeIDMap", cnt.CpuIds[0])
		}
		return nodeID, nil
	}

	//  resources that are considered host-level resources (like ephemeral storage, devices with excludePolicy:"true", etc.),
	//  are not considered for NUMA placement and they will not have NUMA topology info thus GetNUMAID will return -1
	for _, dev := range cnt.Devices {
		if len(dev.DeviceIds) == 0 {
			continue
		}
		nodeIDs := numaloclib.GetNUMAIDs(dev.Topology)
		if len(nodeIDs) == 0 {
			continue
		}

		if len(nodeIDs) > 1 {
			// should never happen on singleNUMANode policy
			return -1, fmt.Errorf("multiple NUMA nodes found for container %s", cnt.Name)
		}
		return nodeIDs[0], nil
	}

	for _, mem := range cnt.Memory {
		nodeIDs := numaloclib.GetNUMAIDs(mem.Topology)
		if len(nodeIDs) == 0 {
			continue
		}

		if len(nodeIDs) > 1 {
			// should never happen on singleNUMANode policy
			return -1, fmt.Errorf("multiple NUMA nodes found for container %s", cnt.Name)
		}
		return nodeIDs[0], nil
	}
	return -1, nil
}
