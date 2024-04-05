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

package config

import (
	"fmt"
	"time"

	"k8s.io/klog/v2"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podres/middleware/sharedcpuspool"
)

type configMap map[string]interface{}

func (cm configMap) String(key string, out *string) error {
	val, ok := cm[key]
	if !ok {
		return nil
	}
	s, ok := val.(string)
	if !ok {
		return fmt.Errorf("key %q has non-string value %T", key, val)
	}
	*out = s
	return nil
}

func (cm configMap) Int(key string, out *int) error {
	val, ok := cm[key]
	if !ok {
		return nil
	}
	j, ok := val.(float64)
	if !ok {
		return fmt.Errorf("key %q has non-float64 value representation %T", key, val)
	}
	*out = int(j)
	return nil
}

func (cm configMap) Bool(key string, out *bool) error {
	val, ok := cm[key]
	if !ok {
		return nil
	}
	b, ok := val.(bool)
	if !ok {
		return fmt.Errorf("key %q has non-bool value %T", key, val)
	}
	*out = b
	return nil
}

func (cm configMap) Duration(key string, out *time.Duration) error {
	val, ok := cm[key]
	if !ok {
		return nil
	}
	j, ok := val.(float64)
	if !ok {
		return fmt.Errorf("key %q has non-float64 value representation %T", key, val)
	}
	*out = time.Duration(j)
	return nil
}

func (cm configMap) Value(key string, out_ any) error {
	switch out := out_.(type) {
	case *string:
		return cm.String(key, out)
	case *int:
		return cm.Int(key, out)
	case *bool:
		return cm.Bool(key, out)
	case *time.Duration:
		return cm.Duration(key, out)
	default:
		return fmt.Errorf("unsupported target value type: %T", out)
	}
}

type confBinding struct {
	key string
	out any
}

func dispatchConfObj(obj map[string]interface{}, pArgs *ProgArgs) error {
	var err error
	cm := configMap(obj)

	cbs := []confBinding{
		{key: "global.debug", out: &pArgs.Global.Debug},
		{key: "global.verbose", out: &pArgs.Global.Verbose},
		{key: "global.kubeconfig", out: &pArgs.Global.KubeConfig},
		{key: "nrtUpdater.noPublish", out: &pArgs.NRTupdater.NoPublish},
		{key: "nrtUpdater.oneShot", out: &pArgs.NRTupdater.Oneshot},
		{key: "nrtUpdater.hostname", out: &pArgs.NRTupdater.Hostname},
		{key: "resourceMonitor.namespace", out: &pArgs.Resourcemonitor.Namespace},
		{key: "resourceMonitor.sysfsRoot", out: &pArgs.Resourcemonitor.SysfsRoot},
		{key: "resourceMonitor.refreshNodeResources", out: &pArgs.Resourcemonitor.RefreshNodeResources},
		{key: "resourceMonitor.podSetFingerprint", out: &pArgs.Resourcemonitor.PodSetFingerprint},
		{key: "resourceMonitor.podSetFingerprintMethod", out: &pArgs.Resourcemonitor.PodSetFingerprintMethod},
		{key: "resourceMonitor.exposeTiming", out: &pArgs.Resourcemonitor.ExposeTiming},
		{key: "resourceMonitor.podSetFingerprintStatusFile", out: &pArgs.Resourcemonitor.PodSetFingerprintStatusFile},
		{key: "resourceMonitor.excludeTerminalPods", out: &pArgs.Resourcemonitor.ExcludeTerminalPods},
		{key: "topologyExporter.podResourcesSocketPath", out: &pArgs.RTE.PodResourcesSocketPath},
		{key: "topologyExporter.sleepInterval", out: &pArgs.RTE.SleepInterval},
		{key: "topologyExporter.podReadinessEnable", out: &pArgs.RTE.PodReadinessEnable},
		{key: "topologyExporter.notifyFilePath", out: &pArgs.RTE.NotifyFilePath},
		{key: "topologyExporter.timeUnitToLimitEvents", out: &pArgs.RTE.TimeUnitToLimitEvents},
		{key: "topologyExporter.addNRTOwnerEnable", out: &pArgs.RTE.AddNRTOwnerEnable},
		{key: "topologyExporter.metricsMode", out: &pArgs.RTE.MetricsMode},
		{key: "topologyExporter.metricsPort", out: &pArgs.RTE.MetricsPort},
		{key: "topologyExporter.MetricsAddress", out: &pArgs.RTE.MetricsAddress},
		{key: "topologyExporter.metricsTLS.certsDir", out: &pArgs.RTE.MetricsTLSCfg.CertsDir},
		{key: "topologyExporter.metricsTLS.certFile", out: &pArgs.RTE.MetricsTLSCfg.CertFile},
		{key: "topologyExporter.metricsTLS.keyFile", out: &pArgs.RTE.MetricsTLSCfg.KeyFile},
		{key: "topologyExporter.metricsTLS.wantCliAuth", out: &pArgs.RTE.MetricsTLSCfg.WantCliAuth},
	}

	for _, cb := range cbs {
		err = cm.Value(cb.key, cb.out)
		if err != nil {
			return err
		}
	}

	// special cases
	var refCnt string
	err = cm.String("topologyExporter.referenceContainer", &refCnt)
	if err != nil {
		return err
	}
	pArgs.RTE.ReferenceContainer, err = sharedcpuspool.ContainerIdentFromString(refCnt)
	if err != nil {
		return err
	}
	if pArgs.Global.Debug {
		klog.Infof("reference container: %+v", pArgs.RTE.ReferenceContainer)
	}

	var maxEventsPerTimeUnit int = -1
	err = cm.Int("topologyExporter.maxEventPerTimeUnit", &maxEventsPerTimeUnit)
	if err != nil {
		return err
	}
	if maxEventsPerTimeUnit != -1 {
		pArgs.RTE.MaxEventsPerTimeUnit = int64(maxEventsPerTimeUnit)
	}

	return nil
}
