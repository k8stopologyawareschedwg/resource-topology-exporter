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
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/test/e2e/framework"

	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
	topologyclientset "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/clientset/versioned"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/nrtupdater"

	e2enodes "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/nodes"
	e2enodetopology "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/nodetopology"
	e2epods "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/pods"
	e2ertepod "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/pods/rtepod"
	e2etestconsts "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/testconsts"
	e2etestenv "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/testenv"
)

var _ = ginkgo.Describe("[RTE][InfraConsuming] Resource topology exporter", func() {
	var (
		initialized         bool
		timeout             time.Duration
		topologyClient      *topologyclientset.Clientset
		topologyUpdaterNode *v1.Node
		workerNodes         []v1.Node
	)

	f := framework.NewDefaultFramework("rte")

	ginkgo.BeforeEach(func() {
		var err error

		if !initialized {
			timeout, err = time.ParseDuration(e2etestenv.GetPollInterval())
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			// wait interval exactly multiple of the poll interval makes the test racier and less robust, so
			// add a little skew. We pick 1 second randomly, but the idea is that small (2, 3, 5) multipliers
			// should again not cause a total multiple of the poll interval.
			timeout += 1 * time.Second

			topologyClient, err = topologyclientset.NewForConfig(f.ClientConfig())
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			workerNodes, err = e2enodes.GetWorkerNodes(f)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			// pick any worker node. The (implicit, TODO: make explicit) assumption is
			// the daemonset runs on CI on all the worker nodes.
			topologyUpdaterNode = &workerNodes[0]
			gomega.Expect(topologyUpdaterNode).NotTo(gomega.BeNil())

			// during the e2e tests we expect changes on the node topology.
			// but in an environment with multiple worker nodes, we might be looking at the wrong node.
			// thus, we assign a unique label to the picked worker node
			// and making sure to deploy the pod on it during the test using nodeSelector
			err = e2enodes.LabelNode(f, topologyUpdaterNode, map[string]string{e2etestconsts.TestNodeLabel: ""})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			initialized = true
		}
	})

	ginkgo.Context("with cluster configured", func() {
		ginkgo.It("[StateDirectories] it should react to pod changes using the smart poller", func() {
			nodes, err := e2enodes.FilterNodesWithEnoughCores(workerNodes, "1000m")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			if len(nodes) < 1 {
				ginkgo.Skip("not enough allocatable cores for this test")
			}

			initialNodeTopo := e2enodetopology.GetNodeTopology(topologyClient, topologyUpdaterNode.Name)
			ginkgo.By("creating a pod consuming the shared pool")
			sleeperPod := e2epods.MakeGuaranteedSleeperPod("1000m")
			defer e2epods.Cooldown(f)

			stopChan := make(chan struct{})
			doneChan := make(chan struct{})
			started := false

			go func() {
				defer ginkgo.GinkgoRecover()

				<-stopChan
				podMap := make(map[string]*v1.Pod)
				pod := f.PodClient().CreateSync(sleeperPod)
				podMap[pod.Name] = pod
				e2epods.DeletePodsAsync(f, podMap)
				doneChan <- struct{}{}
			}()

			ginkgo.By("getting the updated topology")
			var finalNodeTopo *v1alpha1.NodeResourceTopology
			gomega.Eventually(func() bool {
				if !started {
					stopChan <- struct{}{}
					started = true
				}

				finalNodeTopo, err = topologyClient.TopologyV1alpha1().NodeResourceTopologies().Get(context.TODO(), topologyUpdaterNode.Name, metav1.GetOptions{})
				if err != nil {
					framework.Logf("failed to get the node topology resource: %v", err)
					return false
				}
				if finalNodeTopo.ObjectMeta.ResourceVersion == initialNodeTopo.ObjectMeta.ResourceVersion {
					framework.Logf("resource %s not yet updated - resource version not bumped", topologyUpdaterNode.Name)
					return false
				}
				reason, ok := finalNodeTopo.Annotations[nrtupdater.AnnotationRTEUpdate]
				if !ok {
					framework.Logf("resource %s missing annotation!", topologyUpdaterNode.Name)
					return false
				}
				return reason == nrtupdater.RTEUpdateReactive
			}, 5*time.Second, 1*time.Second).Should(gomega.BeTrue(), "didn't get updated node topology info")
			ginkgo.By("checking the topology was updated for the right reason")

			<-doneChan

			gomega.Expect(finalNodeTopo.Annotations).ToNot(gomega.BeNil(), "missing annotations entirely")
			reason := finalNodeTopo.Annotations[nrtupdater.AnnotationRTEUpdate]
			gomega.Expect(reason).To(gomega.Equal(nrtupdater.RTEUpdateReactive), "update reason error: expected %q got %q", nrtupdater.RTEUpdateReactive, reason)
		})

		ginkgo.It("[NotificationFile] it should react to pod changes using the smart poller with notification file", func() {
			initialNodeTopo := e2enodetopology.GetNodeTopology(topologyClient, topologyUpdaterNode.Name)

			stopChan := make(chan struct{})
			doneChan := make(chan struct{})
			started := false

			go func() {
				defer ginkgo.GinkgoRecover()

				<-stopChan
				rtePod, err := e2epods.GetPodOnNode(f, topologyUpdaterNode.Name, e2etestenv.GetNamespaceName(), e2etestenv.RTELabelName)
				framework.ExpectNoError(err)

				rteContainerName, err := e2ertepod.FindRTEContainerName(rtePod)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())

				rteNotifyFilePath, err := e2ertepod.FindNotificationFilePath(rtePod)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())

				execCommandInContainer(f, rtePod.Namespace, rtePod.Name, rteContainerName, "/bin/touch", rteNotifyFilePath)
				doneChan <- struct{}{}
			}()

			ginkgo.By("getting the updated topology")
			var err error
			var finalNodeTopo *v1alpha1.NodeResourceTopology
			gomega.Eventually(func() bool {
				if !started {
					stopChan <- struct{}{}
					started = true
				}

				finalNodeTopo, err = topologyClient.TopologyV1alpha1().NodeResourceTopologies().Get(context.TODO(), topologyUpdaterNode.Name, metav1.GetOptions{})
				if err != nil {
					framework.Logf("failed to get the node topology resource: %v", err)
					return false
				}
				if finalNodeTopo.ObjectMeta.ResourceVersion == initialNodeTopo.ObjectMeta.ResourceVersion {
					framework.Logf("resource %s not yet updated - resource version not bumped", topologyUpdaterNode.Name)
					return false
				}
				reason, ok := finalNodeTopo.Annotations[nrtupdater.AnnotationRTEUpdate]
				if !ok {
					framework.Logf("resource %s missing annotation!", topologyUpdaterNode.Name)
					return false
				}
				return reason == nrtupdater.RTEUpdateReactive
			}, 5*time.Second, 1*time.Second).Should(gomega.BeTrue(), "didn't get updated node topology info")
			ginkgo.By("checking the topology was updated for the right reason")

			<-doneChan

			gomega.Expect(finalNodeTopo.Annotations).ToNot(gomega.BeNil(), "missing annotations entirely")
			reason := finalNodeTopo.Annotations[nrtupdater.AnnotationRTEUpdate]
			gomega.Expect(reason).To(gomega.Equal(nrtupdater.RTEUpdateReactive), "update reason error: expected %q got %q", nrtupdater.RTEUpdateReactive, reason)
		})

	})
})

func execCommandInContainer(f *framework.Framework, namespace, podName, containerName string, cmd ...string) string {
	stdout, stderr, err := f.ExecWithOptions(framework.ExecOptions{
		Command:            cmd,
		Namespace:          namespace,
		PodName:            podName,
		ContainerName:      containerName,
		Stdin:              nil,
		CaptureStdout:      true,
		CaptureStderr:      true,
		PreserveWhitespace: false,
	})
	framework.Logf("Exec stderr: %q", stderr)
	framework.ExpectNoError(err, "failed to execute command in namespace %v pod %v, container %v: %v", namespace, podName, containerName, err)
	return stdout
}
