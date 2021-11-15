package nrtupdater

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	v1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
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

func NewNRTUpdater(args Args, policy string) (*NRTUpdater, error) {
	te := &NRTUpdater{
		args:     args,
		tmPolicy: policy,
	}
	return te, nil
}

func (te *NRTUpdater) Update(info MonitorInfo) error {
	klog.V(3).Infof("update: sending zone: '%s'", utils.Dump(info.Zones))

	if te.args.NoPublish {
		return nil
	}

	cli, err := GetTopologyClient("")
	if err != nil {
		return err
	}

	nrt, err := cli.TopologyV1alpha1().NodeResourceTopologies("").Get(context.TODO(), te.args.Hostname, metav1.GetOptions{})
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

		nrtCreated, err := cli.TopologyV1alpha1().NodeResourceTopologies("").Create(context.TODO(), &nrtNew, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("update failed to create v1alpha1.NodeResourceTopology!:%v", err)
		}
		klog.V(2).Infof("update created CRD instance: %v", utils.Dump(nrtCreated))
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

	nrtUpdated, err := cli.TopologyV1alpha1().NodeResourceTopologies("").Update(context.TODO(), nrtMutated, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update failed to update v1alpha1.NodeResourceTopology!:%v", err)
	}
	klog.V(3).Infof("update changed CRD instance: %v", nrtUpdated)
	return nil
}

func (te *NRTUpdater) Run(infoChannel <-chan MonitorInfo) chan<- struct{} {
	done := make(chan struct{})
	go func() {
		for {
			select {
			case info := <-infoChannel:
				tsBegin := time.Now()
				if err := te.Update(info); err != nil {
					klog.Warning("failed to update: %v", err)
				}
				tsEnd := time.Now()

				tsDiff := tsEnd.Sub(tsBegin)
				prometheus.UpdateOperationDelayMetric("node_resource_object_update", RTEUpdateReactive, float64(tsDiff.Milliseconds()))
				if te.args.Oneshot {
					break
				}
			case <-done:
				klog.Infof("update stop at %v", time.Now())
				break
			}
		}
	}()
	return done
}
