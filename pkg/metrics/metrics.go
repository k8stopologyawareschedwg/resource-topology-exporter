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

package metrics

import (
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var nodeName string

var (
	PodResourceApiCallsFailure = promauto.With(ctrlmetrics.Registry).NewCounterVec(prometheus.CounterOpts{
		Name: "rte_podresource_api_call_failures_total",
		Help: "The total number of podresource api calls that failed by the updater",
	}, []string{"node", "function_name"})

	NodeResourceTopologyWrites = promauto.With(ctrlmetrics.Registry).NewCounterVec(prometheus.CounterOpts{
		Name: "rte_noderesourcetopology_writes_total",
		Help: "The total number of NodeResourceTopology writes",
	}, []string{"node", "operation", "trigger"})

	OperationDelay = promauto.With(ctrlmetrics.Registry).NewGaugeVec(prometheus.GaugeOpts{
		Name: "rte_operation_delay_milliseconds",
		Help: "The latency between exporting stages, milliseconds",
	}, []string{"node", "operation_name", "trigger"})

	WakeupDelay = promauto.With(ctrlmetrics.Registry).NewGaugeVec(prometheus.GaugeOpts{
		Name: "rte_wakeup_delay_milliseconds",
		Help: "The wakeup delay of the monitor code, milliseconds",
	}, []string{"node", "trigger"})

	NodeResourceTopologyPatchFailure = promauto.With(ctrlmetrics.Registry).NewCounterVec(prometheus.CounterOpts{
		Name: "rte_noderesourcetopology_patch_failures_total",
		Help: "The total number of times the NodeResourceTopology patching failed",
	}, []string{"node", "trigger"})

	NodeResourceTopologyPatchSizeRatio = promauto.With(ctrlmetrics.Registry).NewHistogramVec(prometheus.HistogramOpts{
		Name:    "rte_noderesourcetopology_patch_size_ratio",
		Help:    "The ratio of patch size to full object size (0.0 to 1.0)",
		Buckets: []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0},
	}, []string{"node"})
)

func UpdateNodeResourceTopologyWritesMetric(operation, trigger string) {
	NodeResourceTopologyWrites.With(prometheus.Labels{
		"node":      nodeName,
		"operation": operation,
		"trigger":   trigger,
	}).Inc()
}

func UpdatePodResourceApiCallsFailuresMetric(funcName string) {
	PodResourceApiCallsFailure.With(prometheus.Labels{
		"node":          nodeName,
		"function_name": funcName,
	}).Inc()
}

func UpdateOperationDelayMetric(opName, trigger string, operationDelay float64) {
	OperationDelay.With(prometheus.Labels{
		"node":           nodeName,
		"operation_name": opName,
		"trigger":        trigger,
	}).Set(operationDelay)
}

func UpdateWakeupDelayMetric(trigger string, wakeupDelay float64) {
	WakeupDelay.With(prometheus.Labels{
		"node":    nodeName,
		"trigger": trigger,
	}).Set(wakeupDelay)
}

func UpdateNodeResourceTopologyPatchFailuresMetric(trigger string) {
	NodeResourceTopologyPatchFailure.With(prometheus.Labels{
		"node":    nodeName,
		"trigger": trigger,
	}).Inc()
}

func UpdateNodeResourceTopologyPatchSizeRatioMetric(ratio float64) {
	NodeResourceTopologyPatchSizeRatio.With(prometheus.Labels{
		"node": nodeName,
	}).Observe(ratio)
}

func Setup(nname string) error {
	var err error
	var ok bool
	var val string = nname
	if val == "" {
		val, ok = os.LookupEnv("NODE_NAME")
		if !ok {
			val, err = os.Hostname()
		}
	}
	if err != nil {
		return err
	}
	nodeName = val
	return nil
}

// GetNodeName is meant for testing purposes
func GetNodeName() string {
	return nodeName
}
