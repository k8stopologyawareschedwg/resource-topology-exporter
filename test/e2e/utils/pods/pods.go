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
	"os"
	"sync"
	"time"

	"github.com/onsi/ginkgo"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/test/e2e/framework"

	e2etestconsts "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/testconsts"
)

const (
	CentosImage = "quay.io/centos/centos:8"
)

func MakeGuaranteedSleeperPod(cpuLimit string) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sleeper-gu-pod",
		},
		Spec: v1.PodSpec{
			NodeSelector:  map[string]string{e2etestconsts.TestNodeLabel: ""},
			RestartPolicy: v1.RestartPolicyNever,
			Containers: []v1.Container{
				{
					Name:  "sleeper-gu-cnt",
					Image: CentosImage,
					// 1 hour (or >= 1h in general) is "forever" for our purposes
					Command: []string{"/bin/sleep", "1h"},
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							// we use 1 core because that's the minimal meaningful quantity
							v1.ResourceName(v1.ResourceCPU): resource.MustParse(cpuLimit),
							// any random reasonable amount is fine
							v1.ResourceName(v1.ResourceMemory): resource.MustParse("100Mi"),
						},
					},
				},
			},
		},
	}
}

func MakeBestEffortSleeperPod() *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sleeper-be-pod",
		},
		Spec: v1.PodSpec{
			NodeSelector:  map[string]string{e2etestconsts.TestNodeLabel: ""},
			RestartPolicy: v1.RestartPolicyNever,
			Containers: []v1.Container{
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

func DeletePodsAsync(f *framework.Framework, podMap map[string]*v1.Pod) {
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
