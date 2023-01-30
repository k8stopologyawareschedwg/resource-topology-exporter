package nrtupdater

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha2"
	topologyclientset "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/clientset/versioned"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/dump"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/k8sannotations"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/k8shelpers"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podreadiness"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/prometheus"
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

type TMConfig struct {
	Policy string
	Scope  string
}

func (conf TMConfig) IsValid() bool {
	return conf.Policy != "" && conf.Scope != ""
}

type NRTUpdater struct {
	args     Args
	tmPolicy string
	tmConfig TMConfig
	stopChan chan struct{}
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

func NewNRTUpdater(args Args, policy string, tmconf TMConfig) *NRTUpdater {
	return &NRTUpdater{
		args:     args,
		tmPolicy: policy,
		tmConfig: tmconf,
		stopChan: make(chan struct{}),
	}
}

func (te *NRTUpdater) Update(info MonitorInfo) error {
	// early check to avoid creating the client if we can help it
	if te.args.NoPublish {
		return nil
	}
	cli, err := k8shelpers.GetTopologyClient("")
	if err != nil {
		return err
	}
	return te.UpdateWithClient(cli, info)
}

func (te *NRTUpdater) UpdateWithClient(cli topologyclientset.Interface, info MonitorInfo) error {
	klog.V(7).Infof("update: sending zone: %v", dump.Object(info.Zones))

	if te.args.NoPublish {
		return nil
	}

	nrt, err := cli.TopologyV1alpha2().NodeResourceTopologies().Get(context.TODO(), te.args.Hostname, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		nrtNew := v1alpha2.NodeResourceTopology{
			ObjectMeta: metav1.ObjectMeta{
				Name:        te.args.Hostname,
				Annotations: make(map[string]string),
			},
		}
		te.updateNRTInfo(&nrtNew, info)

		nrtCreated, err := cli.TopologyV1alpha2().NodeResourceTopologies().Create(context.TODO(), &nrtNew, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("update failed for NRT instance: %w", err)
		}
		prometheus.UpdateNodeResourceTopologyWritesMetric("create", info.UpdateReason())
		klog.V(2).Infof("nrtupdater created NRT instance: %v", dump.Object(nrtCreated))
		return nil
	}

	if err != nil {
		return err
	}

	nrtMutated := nrt.DeepCopy()
	te.updateNRTInfo(nrtMutated, info)

	nrtUpdated, err := cli.TopologyV1alpha2().NodeResourceTopologies().Update(context.TODO(), nrtMutated, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update failed for NRT instance: %w", err)
	}
	prometheus.UpdateNodeResourceTopologyWritesMetric("update", info.UpdateReason())
	klog.V(7).Infof("nrtupdater changed CRD instance: %v", dump.Object(nrtUpdated))
	return nil
}

func (te *NRTUpdater) updateNRTInfo(nrt *v1alpha2.NodeResourceTopology, info MonitorInfo) {
	nrt.Annotations = mergeAnnotations(nrt.Annotations, info.Annotations)
	nrt.Annotations[k8sannotations.RTEUpdate] = info.UpdateReason()
	nrt.TopologyPolicies = []string{te.tmPolicy}
	nrt.Zones = info.Zones.DeepCopy()
	nrt.Attributes = info.Attributes.DeepCopy()
	nrt.Attributes = append(nrt.Attributes, te.makeAttributes()...)
	// TODO: check for duplicate attributes?
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

func mergeAnnotations(kvs ...map[string]string) map[string]string {
	ret := make(map[string]string)
	for _, kv := range kvs {
		for key, value := range kv {
			ret[key] = value
		}
	}
	return ret
}
