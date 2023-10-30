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
)

var nodeName string

var (
	PodResourceApiCallsFailure = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "rte_podresource_api_call_failures_total",
		Help: "The total number of podresource api calls that failed by the updater",
	}, []string{"node", "function_name"})

	NodeResourceTopologyWrites = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "rte_noderesourcetopology_writes_total",
		Help: "The total number of NodeResourceTopology writes",
	}, []string{"node", "operation", "trigger"})

	OperationDelay = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "rte_operation_delay_milliseconds",
		Help: "The latency between exporting stages, milliseconds",
	}, []string{"node", "operation_name", "trigger"})

	WakeupDelay = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "rte_wakeup_delay_milliseconds",
		Help: "The wakeup delay of the monitor code, milliseconds",
	}, []string{"node", "trigger"})
)

func UpdateNodeResourceTopologyWritesMetric(operation, trigger string) {
	NodeResourceTopologyWrites.With(prometheus.Labels{
		"node":      nodeName,
		"operation": operation,
		"trigger":   trigger,
	}).Inc()
}

func UpdatePodResourceApiCallsFailureMetric(funcName string) {
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
