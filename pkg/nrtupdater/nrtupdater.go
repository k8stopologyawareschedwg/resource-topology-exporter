package nrtupdater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/jsonmergepatch"
	"k8s.io/klog/v2"

	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha2"
	topologyclientset "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/clientset/versioned"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/dump"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/k8sannotations"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/metrics"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podreadiness"
)

const (
	RTEUpdatePeriodic = "periodic"
	RTEUpdateReactive = "reactive"
)

var (
	ErrMissingPreviousNRT = errors.New("missing previous NRT data")
)

// Command line arguments
type Args struct {
	NoPublish   bool   `json:"noPublish,omitempty"`
	Oneshot     bool   `json:"oneShot,omitempty"`
	Hostname    string `json:"hostname,omitempty"`
	KubeConfig  string `json:"kubeConfig,omitempty"`
	PatchMode   bool   `json:"patchMode,omitempty"`
	PatchResync int    `json:"patchResync,omitempty"`
}

func (args Args) Clone() Args {
	return Args{
		NoPublish:   args.NoPublish,
		Oneshot:     args.Oneshot,
		Hostname:    args.Hostname,
		PatchMode:   args.PatchMode,
		PatchResync: args.PatchResync,
	}
}

type TMConfig struct {
	Policy string
	Scope  string
}

func (conf TMConfig) IsValid() bool {
	return conf.Policy != "" && conf.Scope != ""
}

type NRTUpdater struct {
	args       Args
	tmConfig   TMConfig
	stopChan   chan struct{}
	nodeGetter NodeGetter
	nrtCli     topologyclientset.Interface
	sendObject func(context.Context, topologyclientset.Interface, MonitorInfo) (*v1alpha2.NodeResourceTopology, error)
	prevNRT    *v1alpha2.NodeResourceTopology
}

type MonitorInfo struct {
	Timer       bool
	Zones       v1alpha2.ZoneList
	Attributes  v1alpha2.AttributeList
	Annotations map[string]string
}

func (mi MonitorInfo) UpdateReason() string {
	if mi.Timer {
		return RTEUpdatePeriodic
	}
	return RTEUpdateReactive
}

func NewNRTUpdater(nodeGetter NodeGetter, nrtCli topologyclientset.Interface, args Args, tmconf TMConfig) (*NRTUpdater, error) {
	if nrtCli == nil {
		return nil, fmt.Errorf("missing NRT client interface")
	}
	upd := NRTUpdater{
		args:       args,
		tmConfig:   tmconf,
		stopChan:   make(chan struct{}),
		nodeGetter: nodeGetter,
		nrtCli:     nrtCli,
	}
	if args.PatchMode {
		klog.Infof("operation mode: patch")
		upd.sendObject = upd.sendObjectPatch
	} else {
		klog.Infof("operation mode: get+update")
		upd.sendObject = upd.sendObjectUpdate
	}
	return &upd, nil
}

func (te *NRTUpdater) Update(ctx context.Context, info MonitorInfo) error {
	return te.sendData(ctx, te.nrtCli, info)
}

func (te *NRTUpdater) Stop() {
	te.stopChan <- struct{}{}
}

func (te *NRTUpdater) Run(infoChannel <-chan MonitorInfo, condChan chan v1.PodCondition) {
	for {
		select {
		case info := <-infoChannel:
			tsBegin := time.Now()
			condStatus := v1.ConditionTrue
			if err := te.Update(context.Background(), info); err != nil {
				klog.Warningf("failed to update: %v", err)
				condStatus = v1.ConditionFalse
			}
			tsEnd := time.Now()

			tsDiff := tsEnd.Sub(tsBegin)
			metrics.UpdateOperationDelayMetric("node_resource_object_update", RTEUpdateReactive, float64(tsDiff.Milliseconds()))
			if te.args.Oneshot {
				break
			}
			podreadiness.SetCondition(condChan, podreadiness.NodeTopologyUpdated, condStatus)
		case <-te.stopChan:
			klog.Infof("update stop at %v", time.Now())
			return
		}
	}
}

func (te *NRTUpdater) sendData(ctx context.Context, cli topologyclientset.Interface, info MonitorInfo) error {
	klog.V(7).Infof("update: sending zone: %v", dump.Object(info.Zones))
	if te.args.NoPublish {
		return nil
	}
	_, err := te.sendObject(ctx, cli, info)
	return err
}

type NRTPatchInfo struct {
	Patch        []byte
	FullObjBytes int
}

func (pi NRTPatchInfo) SizeRatio() float64 {
	if pi.FullObjBytes == 0 {
		return 1.0
	}
	return float64(len(pi.Patch)) / float64(pi.FullObjBytes)
}

func MakeNRTPatch(nrtOld, nrtNew *v1alpha2.NodeResourceTopology) (NRTPatchInfo, string, error) {
	nrtOldJSON, err := json.Marshal(nrtOld)
	if err != nil {
		return NRTPatchInfo{}, "marshal_previous", err
	}

	nrtNewJSON, err := json.Marshal(nrtNew)
	if err != nil {
		return NRTPatchInfo{}, "marshal_current", err
	}

	patch, err := jsonmergepatch.CreateThreeWayJSONMergePatch(nrtOldJSON, nrtNewJSON, nrtOldJSON)
	if err != nil {
		return NRTPatchInfo{}, "make_patch", err
	}
	return NRTPatchInfo{
		Patch:        patch,
		FullObjBytes: len(nrtNewJSON),
	}, "", nil
}

func (te *NRTUpdater) patchNRT(ctx context.Context, cli topologyclientset.Interface, info MonitorInfo) (*v1alpha2.NodeResourceTopology, error) {
	if te.prevNRT == nil {
		// we intentionally don't log nor record a metric because this can happen in the regular flow and it's a benign error.
		return nil, ErrMissingPreviousNRT
	}

	// make the patch ignore metadata, otherwise the patch will attempt
	// to remove them, and the apiserver will (correctly) refuse it,
	// falling back to the update path
	nrtNew := te.prevNRT.DeepCopy()
	te.updateNRTInfo(nrtNew, info)
	te.updateOwnerReferences(ctx, nrtNew)

	patchInfo, reason, err := MakeNRTPatch(te.prevNRT, nrtNew)
	if err != nil {
		metrics.UpdateNodeResourceTopologyPatchFailuresMetric(reason)
		klog.Infof("failed to create a patch for the APIServer: %v", err)
		return nil, err
	}

	ratio := patchInfo.SizeRatio()
	klog.V(7).Infof("nrtupdater patch size %d bytes, full object %d bytes, ratio %.2f", len(patchInfo.Patch), patchInfo.FullObjBytes, ratio)
	metrics.UpdateNodeResourceTopologyPatchSizeRatioMetric(ratio)

	// The NodeResourceTopology API types lack patchStrategy/patchMergeKey struct tags,
	// so strategic merge patch would fall back to JSON merge patch behavior anyway.
	// We use MergePatchType to match the actual semantics.
	nrtUpdated, err := cli.TopologyV1alpha2().NodeResourceTopologies().Patch(ctx, te.prevNRT.Name, types.MergePatchType, patchInfo.Patch, metav1.PatchOptions{})
	if err != nil {
		metrics.UpdateNodeResourceTopologyPatchFailuresMetric("send_patch")
		klog.Infof("failed to send a patch to the APIServer: %v", err)
		return nil, err
	}

	te.prevNRT = nrtUpdated
	return nrtUpdated, nil
}

func (te *NRTUpdater) sendObjectPatch(ctx context.Context, cli topologyclientset.Interface, info MonitorInfo) (*v1alpha2.NodeResourceTopology, error) {
	nrtObj, err := te.patchNRT(ctx, cli, info)
	if err != nil {
		nrtObj, err = te.sendObjectUpdate(ctx, cli, info)
		if err != nil {
			return nil, err
		}
		te.prevNRT = nrtObj
		return nrtObj, nil
	}
	return nrtObj, nil
}

func (te *NRTUpdater) sendObjectUpdate(ctx context.Context, cli topologyclientset.Interface, info MonitorInfo) (*v1alpha2.NodeResourceTopology, error) {
	nrt, err := cli.TopologyV1alpha2().NodeResourceTopologies().Get(ctx, te.args.Hostname, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		nrtNew := v1alpha2.NodeResourceTopology{
			ObjectMeta: metav1.ObjectMeta{
				Name:        te.args.Hostname,
				Annotations: make(map[string]string),
			},
		}
		te.updateNRTInfo(&nrtNew, info)
		te.updateOwnerReferences(ctx, &nrtNew)

		nrtCreated, err := cli.TopologyV1alpha2().NodeResourceTopologies().Create(ctx, &nrtNew, metav1.CreateOptions{})
		if err != nil {
			return nil, fmt.Errorf("update failed for NRT instance: %w", err)
		}
		metrics.UpdateNodeResourceTopologyWritesMetric("create", info.UpdateReason())
		klog.V(2).Infof("nrtupdater created NRT instance: %v", dump.Object(nrtCreated))
		return nrtCreated, nil
	}

	if err != nil {
		return nil, err
	}

	nrtMutated := nrt.DeepCopy()
	te.updateNRTInfo(nrtMutated, info)
	te.updateOwnerReferences(ctx, nrtMutated)

	nrtUpdated, err := cli.TopologyV1alpha2().NodeResourceTopologies().Update(ctx, nrtMutated, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("update failed for NRT instance: %w", err)
	}
	metrics.UpdateNodeResourceTopologyWritesMetric("update", info.UpdateReason())
	klog.V(7).Infof("nrtupdater changed CRD instance: %v", dump.Object(nrtUpdated))
	return nrtUpdated, nil
}

func (te *NRTUpdater) updateNRTInfo(nrt *v1alpha2.NodeResourceTopology, info MonitorInfo) {
	nrt.Annotations = k8sannotations.Merge(nrt.Annotations, info.Annotations)
	nrt.Annotations[k8sannotations.RTEUpdate] = info.UpdateReason()
	nrt.Zones = info.Zones.DeepCopy()
	nrt.Attributes = info.Attributes.DeepCopy()
	nrt.Attributes = append(nrt.Attributes, te.makeAttributes()...)
	// TODO: check for duplicate attributes?
}

// updateOwnerReferences ensure nrt.OwnerReferences include a reference to the Node with the same name as the NRT
//
// Check nrt.OwnerReferences for Node references and update it so it has only one Node reference,
// the one to the Node with the same name as the NRT.
func (te *NRTUpdater) updateOwnerReferences(ctx context.Context, nrt *v1alpha2.NodeResourceTopology) {
	node, err := te.nodeGetter.Get(ctx, nrt.Name, metav1.GetOptions{})
	if err != nil {
		if errors.Is(err, NotConfigured) {
			return
		}
		klog.V(7).Infof("nrtupdater unable to get Node %s. Can't add Owner reference. error: %v", nrt.Name, err)
		return
	}
	nodeReference := metav1.OwnerReference{
		APIVersion: "v1",
		Kind:       "Node",
		Name:       node.Name,
		UID:        node.UID,
	}

	nrt.OwnerReferences = []metav1.OwnerReference{nodeReference}
}

func (te *NRTUpdater) makeAttributes() v1alpha2.AttributeList {
	return v1alpha2.AttributeList{
		{
			Name:  "topologyManagerScope",
			Value: te.tmConfig.Scope,
		},
		{
			Name:  "topologyManagerPolicy",
			Value: te.tmConfig.Policy,
		},
	}
}
