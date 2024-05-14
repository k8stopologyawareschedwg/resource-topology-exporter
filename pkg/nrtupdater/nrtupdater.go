package nrtupdater

import (
	"context"
	"errors"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha2"
	topologyclientset "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/clientset/versioned"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/dump"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/k8sannotations"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/metrics"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podreadiness"
)

type NRTUpdater struct {
	args       Args
	tmConfig   TMConfig
	stopChan   chan struct{}
	nodeGetter NodeGetter
	nrtCli     topologyclientset.Interface
}

func NewNRTUpdater(nodeGetter NodeGetter, nrtCli topologyclientset.Interface, args Args, tmconf TMConfig) (*NRTUpdater, error) {
	if nrtCli == nil {
		return nil, fmt.Errorf("missing NRT client interface")
	}
	return &NRTUpdater{
		args:       args,
		tmConfig:   tmconf,
		stopChan:   make(chan struct{}),
		nodeGetter: nodeGetter,
		nrtCli:     nrtCli,
	}, nil
}

func (te *NRTUpdater) Update(ctx context.Context, info MonitorInfo) error {
	return te.updateWithClient(ctx, te.nrtCli, info)
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

func (te *NRTUpdater) updateWithClient(ctx context.Context, cli topologyclientset.Interface, info MonitorInfo) error {
	klog.V(7).Infof("update: sending zone: %v", dump.Object(info.Zones))

	if te.args.NoPublish {
		return nil
	}

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
			return fmt.Errorf("update failed for NRT instance: %w", err)
		}
		metrics.UpdateNodeResourceTopologyWritesMetric("create", info.UpdateReason())
		klog.V(2).Infof("nrtupdater created NRT instance: %v", dump.Object(nrtCreated))
		return nil
	}

	if err != nil {
		return err
	}

	nrtMutated := nrt.DeepCopy()
	te.updateNRTInfo(nrtMutated, info)
	te.updateOwnerReferences(ctx, nrtMutated)

	nrtUpdated, err := cli.TopologyV1alpha2().NodeResourceTopologies().Update(context.TODO(), nrtMutated, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update failed for NRT instance: %w", err)
	}
	metrics.UpdateNodeResourceTopologyWritesMetric("update", info.UpdateReason())
	klog.V(7).Infof("nrtupdater changed CRD instance: %v", dump.Object(nrtUpdated))
	return nil
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
