/*
Copyright 2024 The Kubernetes Authors.

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

package k8sres

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha2"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/convert"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/dump"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/metrics"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podreadiness"
	resup "github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/resourceupdater"
)

type K8SResUpdater struct {
	args     resup.Args
	tmConfig resup.TMConfig
	stopChan chan struct{}
	cli      kubernetes.Interface
}

func NewK8SResUpdater(cli kubernetes.Interface, args resup.Args, tmconf resup.TMConfig) (*K8SResUpdater, error) {
	if cli == nil {
		return nil, fmt.Errorf("missing NRT client interface")
	}
	return &K8SResUpdater{
		args:     args,
		tmConfig: tmconf,
		stopChan: make(chan struct{}),
		cli:      cli,
	}, nil
}

func (te *K8SResUpdater) Update(ctx context.Context, info resup.MonitorInfo) error {
	klog.V(7).Infof("update: sending zone: %v", dump.Object(info.Zones))

	if te.args.NoPublish {
		return nil
	}

	nrt := v1alpha2.NodeResourceTopology{
		ObjectMeta: metav1.ObjectMeta{
			Name:        te.args.Hostname,
			Annotations: map[string]string{},
		},
	}
	info.UpdateNRT(&nrt, te.tmConfig)
	res := convert.NodeResourceTopologyToK8SResourceSlice(&nrt)

	obj, err := te.cli.ResourceV1alpha2().ResourceSlices().Get(ctx, te.args.Hostname, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		resCreated, err := cli.ResourceV1alpha2().ResourceSlices().Create(ctx, res, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("update failed for NRT instance: %w", err)
		}
		metrics.UpdateNodeResourceTopologyWritesMetric("create", info.UpdateReason())
		klog.V(2).Infof("resourceupdater created K8S ResourceSlice instance: %v", dump.Object(res))
		return nil
	}

	if err != nil {
		return err
	}

	resMutated := obj.DeepCopy()
	// TODO

	resUpdated, err := cli.ResourceV1alpha2().ResourceSlice().Update(ctx, resMutated, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update failed for K8S ResourceSlice instance: %w", err)
	}
	metrics.UpdateNodeResourceTopologyWritesMetric("update", info.UpdateReason())
	klog.V(7).Infof("resourceupdater changed K8S ResourceSlice instance: %v", dump.Object(resUpdated))
	return nil

}

func (te *K8SResUpdater) Stop() {
	te.stopChan <- struct{}{}
}

func (te *K8SResUpdater) Run(infoChannel <-chan resup.MonitorInfo, condChan chan v1.PodCondition) {
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
			metrics.UpdateOperationDelayMetric("node_resource_object_update", resup.RTEUpdateReactive, float64(tsDiff.Milliseconds()))
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
