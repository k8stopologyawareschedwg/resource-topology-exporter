/*
Copyright 2022 The Kubernetes Authors.

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

// rtepod includes utilities to introspect the RTE pod - and the RTE pod only
package rtepod

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"
)

const (
	rteExecutable = "resource-topology-exporter"

	notificationFileOption = "--notify-file"
)

func FindMetricsPort(rtePod *corev1.Pod) (int, error) {
	for idx := 0; idx < len(rtePod.Spec.Containers); idx++ {
		cnt := rtePod.Spec.Containers[idx] // shortcut
		if !isRTEContainer(cnt) {
			continue
		}

		for _, envVar := range cnt.Env {
			if envVar.Name == "METRICS_PORT" {
				val, err := strconv.Atoi(envVar.Value)
				if err != nil {
					return 0, err
				}
				return val, nil
			}
		}
	}
	return 0, fmt.Errorf("cannot find METRICS_PORT environment variable")
}

func FindMetricsAddress(rtePod *corev1.Pod) (string, error) {
	for idx := 0; idx < len(rtePod.Spec.Containers); idx++ {
		cnt := rtePod.Spec.Containers[idx] // shortcut
		if !isRTEContainer(cnt) {
			continue
		}

		for _, envVar := range cnt.Env {
			if envVar.Name == "METRICS_ADDRESS" {
				return envVar.Value, nil
			}
		}
	}
	return "", fmt.Errorf("cannot find METRICS_ADDRESS environment variable")
}

func FindRTEContainerName(rtePod *corev1.Pod) (string, error) {
	for idx := 0; idx < len(rtePod.Spec.Containers); idx++ {
		cnt := rtePod.Spec.Containers[idx] // shortcut
		if isRTEContainer(cnt) {
			return cnt.Name, nil
		}
	}
	return "", fmt.Errorf("no container uses %q as command or argument", rteExecutable)
}

func FindNotificationFilePath(ctx context.Context, cli *kubernetes.Clientset, rtePod *corev1.Pod) (string, error) {
	// try CLI args first (legacy)
	for idx := 0; idx < len(rtePod.Spec.Containers); idx++ {
		cnt := rtePod.Spec.Containers[idx] // shortcut
		if len(cnt.Command) > 0 && strings.Contains(cnt.Command[0], rteExecutable) {
			for _, arg := range cnt.Args {
				if strings.Contains(arg, notificationFileOption) {
					return extractOptionValue(arg)
				}
			}
		}
	}
	return findNotifyFilePathFromConfigMap(ctx, cli, rtePod)
}

func findNotifyFilePathFromConfigMap(ctx context.Context, cli *kubernetes.Clientset, rtePod *corev1.Pod) (string, error) {
	for _, vol := range rtePod.Spec.Volumes {
		if vol.ConfigMap == nil {
			continue
		}
		if !strings.Contains(vol.ConfigMap.Name, "daemon") {
			continue
		}
		cm, err := cli.CoreV1().ConfigMaps(rtePod.Namespace).Get(ctx, vol.ConfigMap.Name, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("failed to get configmap %q: %w", vol.ConfigMap.Name, err)
		}
		data, ok := cm.Data["config.yaml"]
		if !ok {
			continue
		}
		var cfg map[string]any
		if err := yaml.Unmarshal([]byte(data), &cfg); err != nil {
			continue
		}
		te, ok := cfg["topologyExporter"].(map[string]any)
		if !ok {
			continue
		}
		notifyPath, ok := te["notifyFilePath"].(string)
		if ok && notifyPath != "" {
			return notifyPath, nil
		}
	}
	return "", fmt.Errorf("cannot find notification file path from CLI args or daemon configmap")
}

func isRTEContainer(cnt corev1.Container) bool {
	// is the command name the one we expect?
	if len(cnt.Command) > 0 && strings.Contains(cnt.Command[0], rteExecutable) {
		return true
	}
	if len(cnt.Args) > 0 && strings.Contains(cnt.Args[0], rteExecutable) {
		return true
	}
	return false
}

func extractOptionValue(keyValue string) (string, error) {
	items := strings.SplitN(keyValue, "=", 2)
	if len(items) != 2 {
		return "", fmt.Errorf("malformed key=value option: %q", keyValue)
	}
	return items[1], nil
}
