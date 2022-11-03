/*
Copyright 2020 The Kubernetes Authors.

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

/*
 * resource-topology-exporter specific tests
 */

package rte

import (
	"context"
	"fmt"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/kubernetes/test/e2e/framework"
	admissionapi "k8s.io/pod-security-admission/api"

	e2enodes "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/nodes"
	e2epods "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/pods"
	e2ertepod "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/pods/rtepod"
	e2etestenv "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/testenv"
)

var _ = ginkgo.Describe("[RTE][Monitoring] metrics", func() {
	var (
		initialized         bool
		rtePod              *corev1.Pod
		metricsPort         int
		workerNodes         []corev1.Node
		topologyUpdaterNode *corev1.Node
	)

	f := framework.NewDefaultFramework("metrics")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	ginkgo.BeforeEach(func() {
		if !initialized {
			var err error
			var pods *corev1.PodList
			sel, err := labels.Parse(fmt.Sprintf("name=%s", e2etestenv.RTELabelName))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			pods, err = f.ClientSet.CoreV1().Pods(e2etestenv.GetNamespaceName()).List(context.TODO(), metav1.ListOptions{LabelSelector: sel.String()})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gomega.Expect(len(pods.Items)).NotTo(gomega.BeZero())
			rtePod = &pods.Items[0]
			metricsPort, err = e2ertepod.FindMetricsPort(rtePod)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			workerNodes, err = e2enodes.GetWorkerNodes(f)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			// pick any worker node. The (implicit, TODO: make explicit) assumption is
			// the daemonset runs on CI on all the worker nodes.
			topologyUpdaterNode = &workerNodes[0]
			gomega.Expect(topologyUpdaterNode).NotTo(gomega.BeNil())

			initialized = true
		}
	})

	ginkgo.Context("With prometheus endpoint configured", func() {
		ginkgo.It("[EventChain] should have some metrics exported", func() {
			rteContainerName, err := e2ertepod.FindRTEContainerName(rtePod)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			stdout, stderr, err := f.ExecWithOptions(framework.ExecOptions{
				Command:            []string{"curl", fmt.Sprintf("http://127.0.0.1:%d/metrics", metricsPort)},
				Namespace:          rtePod.Namespace,
				PodName:            rtePod.Name,
				ContainerName:      rteContainerName,
				Stdin:              nil,
				CaptureStdout:      true,
				CaptureStderr:      true,
				PreserveWhitespace: false,
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "ExecWithOptions failed with %s:\n%s", err, stderr)
			gomega.Expect(stdout).To(gomega.ContainSubstring("operation_delay"))
			gomega.Expect(stdout).To(gomega.ContainSubstring("wakeup_delay"))
		})

		ginkgo.It("[release] it should report noderesourcetopology writes", func() {
			nodes, err := e2enodes.FilterNodesWithEnoughCores(workerNodes, "1000m")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			if len(nodes) < 1 {
				ginkgo.Skip("not enough allocatable cores for this test")
			}

			dumpPods(f, topologyUpdaterNode.Name, "reference pods")

			sleeperPod := e2epods.MakeGuaranteedSleeperPod("1000m")
			defer e2epods.Cooldown(f)
			pod := f.PodClient().CreateSync(sleeperPod)
			defer e2epods.DeletePodSyncByName(f, pod.Name)

			// now we are sure we have at least a write to be reported
			rteContainerName, err := e2ertepod.FindRTEContainerName(rtePod)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			stdout, stderr, err := f.ExecWithOptions(framework.ExecOptions{
				Command:            []string{"curl", fmt.Sprintf("http://127.0.0.1:%d/metrics", metricsPort)},
				Namespace:          rtePod.Namespace,
				PodName:            rtePod.Name,
				ContainerName:      rteContainerName,
				Stdin:              nil,
				CaptureStdout:      true,
				CaptureStderr:      true,
				PreserveWhitespace: false,
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "ExecWithOptions failed with %s:\n%s", err, stderr)
			gomega.Expect(stdout).To(gomega.ContainSubstring("noderesourcetopology_writes_total"))
		})

	})
})
