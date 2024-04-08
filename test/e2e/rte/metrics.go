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
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/fixture"
	e2enodes "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/nodes"
	e2epods "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/pods"
	e2ertepod "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/pods/rtepod"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/remoteexec"
	e2econsts "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/testconsts"
	e2etestenv "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/testenv"
)

var _ = ginkgo.Describe("[RTE][Monitoring] metrics", func() {
	var (
		initialized         bool
		hasMetrics          bool
		metricsMode         string
		metricsPort         int
		metricsAddress      string
		rtePod              *corev1.Pod
		workerNodes         []corev1.Node
		topologyUpdaterNode *corev1.Node
	)

	f := fixture.New()

	ginkgo.BeforeEach(func() {
		var err error

		nsCleanup, err := f.CreateNamespace("metrics")
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		ginkgo.DeferCleanup(nsCleanup)

		if !initialized {
			hasMetrics, metricsMode = e2etestenv.GetMetricsMode()

			workerNodes, err = e2enodes.GetWorkerNodes(f.K8SCli)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(workerNodes).ToNot(gomega.BeEmpty())

			// pick any worker node. The (implicit, TODO: make explicit) assumption is
			// the daemonset runs on CI on all the worker nodes.
			var hasLabel bool
			topologyUpdaterNode, hasLabel = e2enodes.PickTargetNode(workerNodes)
			gomega.Expect(topologyUpdaterNode).ToNot(gomega.BeNil())

			if !hasLabel {
				// during the e2e tests we expect changes on the node topology.
				// but in an environment with multiple worker nodes, we might be looking at the wrong node.
				// thus, we assign a unique label to the picked worker node
				// and making sure to deploy the pod on it during the test using nodeSelector
				err = e2enodes.LabelNode(f.K8SCli, topologyUpdaterNode, map[string]string{e2econsts.TestNodeLabel: ""})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			}

			var pods *corev1.PodList
			sel, err := labels.Parse(fmt.Sprintf("name=%s", e2etestenv.RTELabelName))
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			pods, err = f.K8SCli.CoreV1().Pods(e2etestenv.GetNamespaceName()).List(context.TODO(), metav1.ListOptions{LabelSelector: sel.String()})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Expect(len(pods.Items)).ToNot(gomega.BeZero())
			rtePod = &pods.Items[0]

			if hasMetrics {
				metricsAddress, err = e2ertepod.FindMetricsAddress(rtePod)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				metricsPort, err = e2ertepod.FindMetricsPort(rtePod)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			}

			initialized = true
		}
	})

	ginkgo.Context("With prometheus endpoint configured", func() {
		ginkgo.BeforeEach(func() {
			if !hasMetrics {
				ginkgo.Skip("metrics disabled")
			}
		})

		ginkgo.It("[EventChain] should have some metrics exported over https", func() {
			if metricsMode != "httptls" {
				ginkgo.Skip("this test requires serving metrics over https")
			}
			rteContainerName, err := e2ertepod.FindRTEContainerName(rtePod)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			cmd := []string{"curl", "-v", "-k", "-L", fmt.Sprintf("https://%s:%d/metrics", metricsAddress, metricsPort)}
			key := client.ObjectKeyFromObject(rtePod)
			klog.Infof("executing cmd: %s on pod %q", cmd, key.String())
			var stdout, stderr []byte
			gomega.Eventually(func() bool {
				var err error
				stdout, stderr, err = remoteexec.CommandOnPod(f.Ctx, f.K8SCli, rtePod, rteContainerName, cmd...)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed exec command on pod. pod=%q; cmd=%q; err=%v; stderr=%q", key.String(), cmd, err, stderr)
				return strings.Contains(string(stdout), "operation_delay") &&
					strings.Contains(string(stdout), "wakeup_delay")
			}).WithPolling(10*time.Second).WithTimeout(3*time.Minute).Should(gomega.BeTrue(), "failed to get metrics from pod\nstdout=%q\nstderr=%q\n", stdout, stderr)
		})

		ginkgo.It("[EventChain] should have some metrics exported over plain http", func() {
			if metricsMode != "http" {
				ginkgo.Skip("this test requires serving metrics over plain http")
			}
			rteContainerName, err := e2ertepod.FindRTEContainerName(rtePod)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			cmd := []string{"curl", "-v", "-L", fmt.Sprintf("http://%s:%d/metrics", metricsAddress, metricsPort)}
			key := client.ObjectKeyFromObject(rtePod)
			klog.Infof("executing cmd: %s on pod %q", cmd, key.String())
			var stdout, stderr []byte
			gomega.Eventually(func() bool {
				var err error
				stdout, stderr, err = remoteexec.CommandOnPod(f.Ctx, f.K8SCli, rtePod, rteContainerName, cmd...)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed exec command on pod. pod=%q; cmd=%q; err=%v; stderr=%q", key.String(), cmd, err, stderr)
				return strings.Contains(string(stdout), "operation_delay") &&
					strings.Contains(string(stdout), "wakeup_delay")
			}).WithPolling(10*time.Second).WithTimeout(3*time.Minute).Should(gomega.BeTrue(), "failed to get metrics from pod\nstdout=%q\nstderr=%q\n", stdout, stderr)
		})
		ginkgo.It("[release] it should report noderesourcetopology writes", func() {
			if metricsMode != "http" {
				ginkgo.Skip("this test requires serving metrics over plain http")
			}
			nodes, err := e2enodes.FilterNodesWithEnoughCores(workerNodes, "1000m")
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			if len(nodes) < 1 {
				ginkgo.Skip("not enough allocatable cores for this test")
			}

			dumpPods(f.K8SCli, topologyUpdaterNode.Name, "reference pods")

			sleeperPod := e2epods.MakeGuaranteedSleeperPod("1000m")
			pod, err := e2epods.CreateSync(f, sleeperPod)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.DeferCleanup(e2epods.DeletePodSyncByName, f, pod.Namespace, pod.Name)

			// now we are sure we have at least a write to be reported
			rteContainerName, err := e2ertepod.FindRTEContainerName(rtePod)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			cmd := []string{"curl", "-v", "-L", fmt.Sprintf("http://%s:%d/metrics", metricsAddress, metricsPort)}
			key := client.ObjectKeyFromObject(rtePod)
			klog.Infof("executing cmd: %s on pod %q", cmd, key.String())
			var stdout, stderr []byte
			gomega.Eventually(func() bool {
				var err error
				stdout, stderr, err = remoteexec.CommandOnPod(f.Ctx, f.K8SCli, rtePod, rteContainerName, cmd...)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed exec command on pod. pod=%q; cmd=%q; err=%v; stderr=%q", key.String(), cmd, err, stderr)

				return strings.Contains(string(stdout), "noderesourcetopology_writes_total")
			}).WithPolling(10*time.Second).WithTimeout(2*time.Minute).Should(gomega.BeTrue(), "failed to get metrics from pod\nstdout=%q\nstderr=%q\n", stdout, stderr)
		})
	})
})
