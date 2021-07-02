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

package utils

import (
         "sync"

         "github.com/onsi/ginkgo"

	 v1 "k8s.io/api/core/v1"
         metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
         "k8s.io/kubernetes/test/e2e/framework"
)

const (
	CentosImage = "quay.io/centos/centos:8"
)

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
