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
 * tests shared with NFD's nfd-topology-updater.
 * please note the test logic itself is shared, the fixture setup/teardown code may be different.
 */

package topology_updater

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/test/e2e/framework"
	k8se2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	admissionapi "k8s.io/pod-security-admission/api"
	"sigs.k8s.io/yaml"

	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha2"
	topologyclientset "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/clientset/versioned"
	e2etestns "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/namespace"
	e2enodes "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/nodes"
	e2enodetopology "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/nodetopology"
	e2epods "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/pods"
	e2econsts "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/testconsts"
	e2etestenv "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/testenv"
)

var _ = ginkgo.Describe("[TopologyUpdater][InfraConsuming] Node topology updater", func() {
	var (
		initialized         bool
		timeout             time.Duration
		tmPolicy            string
		tmScope             string
		topologyClient      *topologyclientset.Clientset
		topologyUpdaterNode *corev1.Node
		workerNodes         []corev1.Node
	)

	f := framework.NewDefaultFramework("topology-updater")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged
	f.SkipNamespaceCreation = true

	ginkgo.BeforeEach(func() {
		var err error

		err = e2etestns.Setup(f)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		if !initialized {
			timeout, err = time.ParseDuration(e2etestenv.GetPollInterval())
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			// wait interval exactly multiple of the poll interval makes the test racier and less robust, so
			// add a little skew. We pick 1 second randomly, but the idea is that small (2, 3, 5) multipliers
			// should again not cause a total multiple of the poll interval.
			timeout += 1 * time.Second

			topologyClient, err = topologyclientset.NewForConfig(f.ClientConfig())
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			workerNodes, err = e2enodes.GetWorkerNodes(f)
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
				err = e2enodes.LabelNode(f, topologyUpdaterNode, map[string]string{e2econsts.TestNodeLabel: ""})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			}

			tmPolicy = e2etestenv.GetTopologyManagerPolicy()
			tmScope = e2etestenv.GetTopologyManagerScope()

			initialized = true
		}
	})

	ginkgo.Context("[release] with cluster configured", func() {
		ginkgo.It("it should not account for any cpus if a container doesn't request exclusive cpus (best effort QOS)", func() {
			devName := e2etestenv.GetDeviceName()

			ginkgo.By("getting the initial topology information")
			initialNodeTopo := e2enodetopology.GetNodeTopologyWithResource(topologyClient, topologyUpdaterNode.Name, devName)
			ginkgo.By("creating a pod consuming resources from the shared, non-exclusive CPU pool (best-effort QoS)")
			sleeperPod := e2epods.MakeBestEffortSleeperPod()

			pod := k8se2epod.NewPodClient(f).CreateSync(sleeperPod)
			ginkgo.DeferCleanup(func(cs clientset.Interface, podNamespace, podName string) error {
				return e2epods.DeletePodSyncByName(cs, podNamespace, podName)
			}, f.ClientSet, pod.Namespace, pod.Name)

			cooldown := 3 * timeout
			ginkgo.By(fmt.Sprintf("getting the updated topology - sleeping for %v", cooldown))
			// the object, hence the resource version must NOT change, so we can only sleep
			time.Sleep(cooldown)
			ginkgo.By("checking the changes in the updated topology - expecting none")
			finalNodeTopo := e2enodetopology.GetNodeTopologyWithResource(topologyClient, topologyUpdaterNode.Name, devName)

			initialAvailRes := e2enodetopology.AvailableResourceListFromNodeResourceTopology(initialNodeTopo)
			finalAvailRes := e2enodetopology.AvailableResourceListFromNodeResourceTopology(finalNodeTopo)
			if len(initialAvailRes) == 0 || len(finalAvailRes) == 0 {
				ginkgo.Fail(fmt.Sprintf("failed to find allocatable resources from node topology initial=%v final=%v", initialAvailRes, finalAvailRes))
			}
			zoneName, resName, cmp, ok := e2enodetopology.CmpAvailableResources(initialAvailRes, finalAvailRes)
			klog.Infof("zone=%q resource=%q cmp=%v ok=%v", zoneName, resName, cmp, ok)
			if !ok {
				ginkgo.Fail(fmt.Sprintf("failed to compare allocatable resources from node topology initial=%v final=%v", initialAvailRes, finalAvailRes))
			}

			// This is actually a workaround.
			// Depending on the (random, by design) order on which ginkgo runs the tests, a test which exclusively allocates CPUs may run before.
			// We cannot (nor should) care about what runs before this test, but we know that this may happen.
			// The proper solution is to wait for ALL the container requesting exclusive resources to be gone before to end the related test.
			// To date, we don't yet have a clean way to wait for these pod (actually containers) to be completely gone
			// (hence, releasing the exclusively allocated CPUs) before to end the test, so this test can run with some leftovers hanging around,
			// which makes the accounting harder. And this is what we handle here.
			isGreaterEqual := (cmp >= 0)
			gomega.Expect(isGreaterEqual).To(gomega.BeTrue(), fmt.Sprintf("final allocatable resources not restored - cmp=%d initial=%v final=%v", cmp, initialAvailRes, finalAvailRes))
		})

		ginkgo.It("it should not account for any cpus if a container doesn't request exclusive cpus (guaranteed QOS, nonintegral cpu request)", func() {
			ginkgo.By("getting the initial topology information")
			initialNodeTopo := e2enodetopology.GetNodeTopology(topologyClient, topologyUpdaterNode.Name)
			ginkgo.By("creating a pod consuming resources from the shared, non-exclusive CPU pool (guaranteed QoS, nonintegral request)")
			sleeperPod := e2epods.MakeGuaranteedSleeperPod("500m")

			pod := k8se2epod.NewPodClient(f).CreateSync(sleeperPod)
			ginkgo.DeferCleanup(func(cs clientset.Interface, podNamespace, podName string) error {
				return e2epods.DeletePodSyncByName(cs, podNamespace, podName)
			}, f.ClientSet, pod.Namespace, pod.Name)

			cooldown := 3 * timeout
			ginkgo.By(fmt.Sprintf("getting the updated topology - sleeping for %v", cooldown))
			// the object, hence the resource version must NOT change, so we can only sleep
			time.Sleep(cooldown)
			ginkgo.By("checking the changes in the updated topology - expecting none")
			finalNodeTopo := e2enodetopology.GetNodeTopology(topologyClient, topologyUpdaterNode.Name)

			initialAllocRes := e2enodetopology.AvailableResourceListFromNodeResourceTopology(initialNodeTopo)
			finalAllocRes := e2enodetopology.AvailableResourceListFromNodeResourceTopology(finalNodeTopo)
			if len(initialAllocRes) == 0 || len(finalAllocRes) == 0 {
				ginkgo.Fail(fmt.Sprintf("failed to find available resources from node topology initial=%v final=%v", initialAllocRes, finalAllocRes))
			}
			zoneName, cmp, ok := e2enodetopology.CmpAvailableCPUs(initialAllocRes, finalAllocRes)
			klog.Infof("zone=%q resource=%q cmp=%v ok=%v", zoneName, corev1.ResourceCPU, cmp, ok)
			if !ok {
				ginkgo.Fail(fmt.Sprintf("failed to compare available resources from node topology initial=%v final=%v", initialAllocRes, finalAllocRes))
			}

			// This is actually a workaround.
			// Depending on the (random, by design) order on which ginkgo runs the tests, a test which exclusively allocates CPUs may run before.
			// We cannot (nor should) care about what runs before this test, but we know that this may happen.
			// The proper solution is to wait for ALL the container requesting exclusive resources to be gone before to end the related test.
			// To date, we don't yet have a clean way to wait for these pod (actually containers) to be completely gone
			// (hence, releasing the exclusively allocated CPUs) before to end the test, so this test can run with some leftovers hanging around,
			// which makes the accounting harder. And this is what we handle here.
			isGreaterEqual := (cmp >= 0)
			gomega.Expect(isGreaterEqual).To(gomega.BeTrue(), fmt.Sprintf("final available resources not restored - cmp=%d initial=%v final=%v", cmp, initialAllocRes, finalAllocRes))
		})

		ginkgo.It("it should account for containers requesting exclusive cpus", func() {
			nodes, err := e2enodes.FilterNodesWithEnoughCores(workerNodes, "1000m")
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			if len(nodes) < 1 {
				ginkgo.Skip("not enough available cores for this test")
			}

			ginkgo.By("getting the initial topology information")
			initialNodeTopo := e2enodetopology.GetNodeTopology(topologyClient, topologyUpdaterNode.Name)
			klog.Infof("initial topology information: %#v", initialNodeTopo)

			ginkgo.By("creating a pod consuming exclusive CPUs")
			sleeperPod := e2epods.MakeGuaranteedSleeperPod("1000m")

			pod := k8se2epod.NewPodClient(f).CreateSync(sleeperPod)
			ginkgo.DeferCleanup(func(cs clientset.Interface, podNamespace, podName string) error {
				return e2epods.DeletePodSyncByName(cs, podNamespace, podName)
			}, f.ClientSet, pod.Namespace, pod.Name)

			ginkgo.By("getting the updated topology")
			var finalNodeTopo *v1alpha2.NodeResourceTopology
			gomega.Eventually(func() bool {
				finalNodeTopo, err = topologyClient.TopologyV1alpha2().NodeResourceTopologies().Get(context.TODO(), topologyUpdaterNode.Name, metav1.GetOptions{})
				if err != nil {
					klog.Infof("failed to get the node topology resource: %v", err)
					return false
				}
				return finalNodeTopo.ObjectMeta.Generation != initialNodeTopo.ObjectMeta.Generation
			}, 5*timeout, 5*time.Second).Should(gomega.BeTrue(), "didn't get updated node topology info")
			klog.Infof("final topology information: %#v", initialNodeTopo)

			ginkgo.By("checking the changes in the updated topology")
			initialAllocRes := e2enodetopology.AvailableResourceListFromNodeResourceTopology(initialNodeTopo)
			finalAllocRes := e2enodetopology.AvailableResourceListFromNodeResourceTopology(finalNodeTopo)
			if len(initialAllocRes) == 0 || len(finalAllocRes) == 0 {
				ginkgo.Fail(fmt.Sprintf("failed to find available resources from node topology initial=%v final=%v", initialAllocRes, finalAllocRes))
			}
			zoneName, resName, isLess := e2enodetopology.LessAvailableResources(initialAllocRes, finalAllocRes)
			klog.Infof("zone=%q resource=%q isLess=%v", zoneName, resName, isLess)
			gomega.Expect(isLess).To(gomega.BeTrue(), fmt.Sprintf("final available resources not decreased - initial=%v final=%v", initialAllocRes, finalAllocRes))
		})

		ginkgo.It("should fill the node resource topologies CR with the data", func() {
			nodeTopology := e2enodetopology.GetNodeTopology(topologyClient, topologyUpdaterNode.Name)
			isValid := e2enodetopology.IsValidNodeTopology(nodeTopology, tmPolicy, tmScope)
			gomega.Expect(isValid).To(gomega.BeTrue(), "received invalid topology:\n%v", toYAML(nodeTopology))
		})
	})
})

func toYAML(obj interface{}) string {
	data, err := yaml.Marshal(obj)
	if err != nil {
		return fmt.Sprintf("<SERIALIZE ERROR: %v>", err)
	}
	return string(data)
}
