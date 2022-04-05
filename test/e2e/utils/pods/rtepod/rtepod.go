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
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
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

func FindRTEContainerName(rtePod *corev1.Pod) (string, error) {
	for idx := 0; idx < len(rtePod.Spec.Containers); idx++ {
		cnt := rtePod.Spec.Containers[idx] // shortcut
		if isRTEContainer(cnt) {
			return cnt.Name, nil
		}
	}
	return "", fmt.Errorf("no container uses %q as command or argument", rteExecutable)
}

func FindNotificationFilePath(rtePod *corev1.Pod) (string, error) {
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
	return "", fmt.Errorf("no container uses %q as an argument option", notificationFileOption)
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
