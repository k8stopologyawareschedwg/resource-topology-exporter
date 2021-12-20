package nrtupdater

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/k8shelpers"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podreadiness"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/prometheus"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/utils"
)

const (
	AnnotationRTEUpdate = "k8stopoawareschedwg/rte-update"
)

const (
	RTEUpdatePeriodic = "periodic"
	RTEUpdateReactive = "reactive"
)

// Command line arguments
type Args struct {
	NoPublish bool
	Oneshot   bool
	Hostname  string
}

type NRTUpdater struct {
	args     Args
	tmPolicy string
	stopChan chan struct{}
}

type MonitorInfo struct {
	Timer bool
	Zones v1alpha1.ZoneList
}

func (mi MonitorInfo) UpdateReason() string {
	if mi.Timer {
		return RTEUpdatePeriodic
	}
	return RTEUpdateReactive
}

func NewNRTUpdater(args Args, policy string) *NRTUpdater {
	return &NRTUpdater{
		args:     args,
		tmPolicy: policy,
		stopChan: make(chan struct{}),
	}
}

func (te *NRTUpdater) Update(info MonitorInfo) error {
	klog.V(3).Infof("update: sending zone: %v", utils.Dump(info.Zones))

	if te.args.NoPublish {
		return nil
	}

	cli, err := k8shelpers.GetTopologyClient("")
	if err != nil {
		return err
	}

	nrt, err := cli.TopologyV1alpha1().NodeResourceTopologies().Get(context.TODO(), te.args.Hostname, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		nrtNew := v1alpha1.NodeResourceTopology{
			ObjectMeta: metav1.ObjectMeta{
				Name: te.args.Hostname,
				Annotations: map[string]string{
					AnnotationRTEUpdate: info.UpdateReason(),
				},
			},
			Zones:            info.Zones,
			TopologyPolicies: []string{te.tmPolicy},
		}

		nrtCreated, err := cli.TopologyV1alpha1().NodeResourceTopologies().Create(context.TODO(), &nrtNew, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("update failed for NRT instance: %v", err)
		}
		klog.V(2).Infof("update created NRT instance: %v", utils.Dump(nrtCreated))
		return nil
	}

	if err != nil {
		return err
	}

	nrtMutated := nrt.DeepCopy()
	if nrtMutated.Annotations == nil {
		nrtMutated.Annotations = make(map[string]string)
	}
	nrtMutated.Annotations[AnnotationRTEUpdate] = info.UpdateReason()
	nrtMutated.Zones = info.Zones

	nrtUpdated, err := cli.TopologyV1alpha1().NodeResourceTopologies().Update(context.TODO(), nrtMutated, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update failed for NRT instance: %v", err)
	}
	klog.V(5).Infof("update changed CRD instance: %v", utils.Dump(nrtUpdated))
	return nil
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
			if err := te.Update(info); err != nil {
				klog.Warningf("failed to update: %v", err)
				condStatus = v1.ConditionFalse
			}
			tsEnd := time.Now()

			tsDiff := tsEnd.Sub(tsBegin)
			prometheus.UpdateOperationDelayMetric("node_resource_object_update", RTEUpdateReactive, float64(tsDiff.Milliseconds()))
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
