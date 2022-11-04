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

package pods

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/labels"

	"github.com/onsi/ginkgo/v2"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/test/e2e/framework"

	e2etestconsts "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/testconsts"
)

const (
	CentosImage  = "quay.io/centos/centos:8"
	RTELabelName = "resource-topology"
)

func MakeGuaranteedSleeperPod(cpuLimit string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sleeper-gu-pod",
		},
		Spec: corev1.PodSpec{
			NodeSelector:  map[string]string{e2etestconsts.TestNodeLabel: ""},
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:  "sleeper-gu-cnt",
					Image: CentosImage,
					// 1 hour (or >= 1h in general) is "forever" for our purposes
					Command: []string{"/bin/sleep", "1h"},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							// we use 1 core because that's the minimal meaningful quantity
							corev1.ResourceName(corev1.ResourceCPU): resource.MustParse(cpuLimit),
							// any random reasonable amount is fine
							corev1.ResourceName(corev1.ResourceMemory): resource.MustParse("100Mi"),
						},
					},
				},
			},
		},
	}
}

func MakeBestEffortSleeperPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sleeper-be-pod",
		},
		Spec: corev1.PodSpec{
			NodeSelector:  map[string]string{e2etestconsts.TestNodeLabel: ""},
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:  "sleeper-be-cnt",
					Image: CentosImage,
					// 1 hour (or >= 1h in general) is "forever" for our purposes
					Command: []string{"/bin/sleep", "1h"},
				},
			},
		},
	}
}

func DeletePodsAsync(f *framework.Framework, podMap map[string]*corev1.Pod) {
	var wg sync.WaitGroup
	for _, pod := range podMap {
		wg.Add(1)
		go func(podNS, podName string) {
			defer ginkgo.GinkgoRecover()
			defer wg.Done()

			DeletePodSyncByName(f, podName)
		}(pod.Namespace, pod.Name)
	}
	wg.Wait()
}

func DeletePodSyncByName(f *framework.Framework, podName string) {
	gp := int64(0)
	delOpts := metav1.DeleteOptions{
		GracePeriodSeconds: &gp,
	}
	f.PodClient().DeleteSync(podName, delOpts, framework.DefaultPodDeletionTimeout)
}

func Cooldown(f *framework.Framework) {
	pollInterval, ok := os.LookupEnv("RTE_POLL_INTERVAL")
	if !ok {
		// nothing to do!
		return
	}
	sleepTime, err := time.ParseDuration(pollInterval)
	if err != nil {
		framework.Logf("WaitPodToBeGone: cannot parse %q: %v", pollInterval, err)
		return
	}

	// wait a little more than a full poll interval to make sure the resourcemonitor catches up
	time.Sleep(sleepTime + 500*time.Millisecond)
}

func GetPodsByLabel(f *framework.Framework, ns, label string) ([]corev1.Pod, error) {
	sel, err := labels.Parse(label)
	if err != nil {
		return nil, err
	}

	pods, err := f.ClientSet.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{LabelSelector: sel.String()})
	if err != nil {
		return nil, err
	}
	return pods.Items, nil
}

func GetPodOnNode(f *framework.Framework, nodeName, namespace, labelName string) (*corev1.Pod, error) {
	framework.Logf("searching for RTE pod in namespace %q with label %q", namespace, labelName)
	pods, err := GetPodsByLabel(f, namespace, fmt.Sprintf("name=%s", labelName))
	if err != nil {
		return nil, err
	}
	if len(pods) == 0 {
		return nil, fmt.Errorf("found no node in %q matching label %q", namespace, labelName)
	}
	for idx := 0; idx < len(pods); idx++ {
		framework.Logf("checking pod %s/%s - is it running on %q?", pods[idx].Namespace, pods[idx].Name, nodeName)
		if pods[idx].Spec.NodeName == nodeName {
			return &pods[idx], nil
		}
	}
	return nil, fmt.Errorf("no pod found running on %q", nodeName)
}

func GetLogsForPod(f *framework.Framework, podNamespace, podName, containerName string) (string, error) {
	previous := false
	request := f.ClientSet.CoreV1().RESTClient().Get().Resource("pods").Namespace(podNamespace).Name(podName).SubResource("log").Param("container", containerName).Param("previous", strconv.FormatBool(previous))
	logs, err := request.Do(context.TODO()).Raw()
	if err != nil {
		return "", err
	}
	if strings.Contains(string(logs), "Internal Error") {
		return "", fmt.Errorf("Fetched log contains \"Internal Error\": %q", string(logs))
	}
	return string(logs), err
}
