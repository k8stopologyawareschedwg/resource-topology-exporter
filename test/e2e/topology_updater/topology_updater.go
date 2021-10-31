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

	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
	topologyclientset "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/clientset/versioned"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/test/e2e/framework"

	e2enodes "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/nodes"
	e2enodetopology "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/nodetopology"
	e2epods "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/pods"
	e2etestenv "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/testenv"
)

var _ = ginkgo.Describe("[TopologyUpdater][InfraConsuming] Node topology updater", func() {
	var (
		initialized         bool
		namespace           string
		tmPolicy            string
		topologyClient      *topologyclientset.Clientset
		topologyUpdaterNode *v1.Node
		workerNodes         []v1.Node
	)

	f := framework.NewDefaultFramework("topology-updater")

	ginkgo.BeforeEach(func() {
		var err error

		if !initialized {
			namespace = e2etestenv.GetNamespaceName()

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
			err = e2enodes.LabelNode(f, topologyUpdaterNode, map[string]string{e2enodes.TestNodeLabel: ""})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			tmPolicy = e2etestenv.GetTopologyManagerPolicy()

			initialized = true
		}
	})

	ginkgo.Context("with cluster configured", func() {
		ginkgo.It("it should not account for any cpus if a container doesn't request exclusive cpus (best effort QOS)", func() {
			ginkgo.By("getting the initial topology information")
			initialNodeTopo := e2enodetopology.GetNodeTopology(topologyClient, topologyUpdaterNode.Name, namespace)
			ginkgo.By("creating a pod consuming resources from the shared, non-exclusive CPU pool (best-effort QoS)")
			sleeperPod := e2epods.MakeBestEffortSleeperPod()

			podMap := make(map[string]*v1.Pod)
			pod := f.PodClient().CreateSync(sleeperPod)
			podMap[pod.Name] = pod
			defer e2epods.DeletePodsAsync(f, podMap)

			cooldown := 30 * time.Second
			ginkgo.By(fmt.Sprintf("getting the updated topology - sleeping for %v", cooldown))
			// the object, hance the resource version must NOT change, so we can only sleep
			time.Sleep(cooldown)
			ginkgo.By("checking the changes in the updated topology - expecting none")
			finalNodeTopo := e2enodetopology.GetNodeTopology(topologyClient, topologyUpdaterNode.Name, namespace)

			initialAvailRes := e2enodetopology.AvailableResourceListFromNodeResourceTopology(initialNodeTopo)
			finalAvailRes := e2enodetopology.AvailableResourceListFromNodeResourceTopology(finalNodeTopo)
			if len(initialAvailRes) == 0 || len(finalAvailRes) == 0 {
				ginkgo.Fail(fmt.Sprintf("failed to find allocatable resources from node topology initial=%v final=%v", initialAvailRes, finalAvailRes))
			}
			zoneName, resName, cmp, ok := e2enodetopology.CmpAvailableResources(initialAvailRes, finalAvailRes)
			framework.Logf("zone=%q resource=%q cmp=%v ok=%v", zoneName, resName, cmp, ok)
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
			initialNodeTopo := e2enodetopology.GetNodeTopology(topologyClient, topologyUpdaterNode.Name, namespace)
			ginkgo.By("creating a pod consuming resources from the shared, non-exclusive CPU pool (guaranteed QoS, nonintegral request)")
			sleeperPod := e2epods.MakeGuaranteedSleeperPod("500m")
			defer e2epods.Cooldown(f)

			podMap := make(map[string]*v1.Pod)
			pod := f.PodClient().CreateSync(sleeperPod)
			podMap[pod.Name] = pod
			defer e2epods.DeletePodsAsync(f, podMap)

			cooldown := 30 * time.Second
			ginkgo.By(fmt.Sprintf("getting the updated topology - sleeping for %v", cooldown))
			// the object, hance the resource version must NOT change, so we can only sleep
			time.Sleep(cooldown)
			ginkgo.By("checking the changes in the updated topology - expecting none")
			finalNodeTopo := e2enodetopology.GetNodeTopology(topologyClient, topologyUpdaterNode.Name, namespace)

			initialAllocRes := e2enodetopology.AvailableResourceListFromNodeResourceTopology(initialNodeTopo)
			finalAllocRes := e2enodetopology.AvailableResourceListFromNodeResourceTopology(finalNodeTopo)
			if len(initialAllocRes) == 0 || len(finalAllocRes) == 0 {
				ginkgo.Fail(fmt.Sprintf("failed to find available resources from node topology initial=%v final=%v", initialAllocRes, finalAllocRes))
			}
			zoneName, resName, cmp, ok := e2enodetopology.CmpAvailableResources(initialAllocRes, finalAllocRes)
			framework.Logf("zone=%q resource=%q cmp=%v ok=%v", zoneName, resName, cmp, ok)
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
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			if len(nodes) < 1 {
				ginkgo.Skip("not enough available cores for this test")
			}

			ginkgo.By("getting the initial topology information")
			initialNodeTopo := e2enodetopology.GetNodeTopology(topologyClient, topologyUpdaterNode.Name, namespace)
			framework.Logf("initial topology information: %#v", initialNodeTopo)

			ginkgo.By("creating a pod consuming exclusive CPUs")
			sleeperPod := e2epods.MakeGuaranteedSleeperPod("1000m")
			defer e2epods.Cooldown(f)

			podMap := make(map[string]*v1.Pod)
			pod := f.PodClient().CreateSync(sleeperPod)
			podMap[pod.Name] = pod
			defer e2epods.DeletePodsAsync(f, podMap)

			ginkgo.By("getting the updated topology")
			var finalNodeTopo *v1alpha1.NodeResourceTopology
			gomega.Eventually(func() bool {
				finalNodeTopo, err = topologyClient.TopologyV1alpha1().NodeResourceTopologies(namespace).Get(context.TODO(), topologyUpdaterNode.Name, metav1.GetOptions{})
				if err != nil {
					framework.Logf("failed to get the node topology resource: %v", err)
					return false
				}
				return finalNodeTopo.ObjectMeta.ResourceVersion != initialNodeTopo.ObjectMeta.ResourceVersion
			}, time.Minute, 5*time.Second).Should(gomega.BeTrue(), "didn't get updated node topology info")
			framework.Logf("final topology information: %#v", initialNodeTopo)

			ginkgo.By("checking the changes in the updated topology")
			initialAllocRes := e2enodetopology.AvailableResourceListFromNodeResourceTopology(initialNodeTopo)
			finalAllocRes := e2enodetopology.AvailableResourceListFromNodeResourceTopology(finalNodeTopo)
			if len(initialAllocRes) == 0 || len(finalAllocRes) == 0 {
				ginkgo.Fail(fmt.Sprintf("failed to find available resources from node topology initial=%v final=%v", initialAllocRes, finalAllocRes))
			}
			zoneName, resName, isLess := e2enodetopology.LessAvailableResources(initialAllocRes, finalAllocRes)
			framework.Logf("zone=%q resource=%q isLess=%v", zoneName, resName, isLess)
			gomega.Expect(isLess).To(gomega.BeTrue(), fmt.Sprintf("final available resources not decreased - initial=%v final=%v", initialAllocRes, finalAllocRes))
		})

		ginkgo.It("should fill the node resource topologies CR with the data", func() {
			nodeTopology := e2enodetopology.GetNodeTopology(topologyClient, topologyUpdaterNode.Name, namespace)
			isValid := e2enodetopology.IsValidNodeTopology(nodeTopology, tmPolicy)
			gomega.Expect(isValid).To(gomega.BeTrue(), "received invalid topology: %v", nodeTopology)
		})
	})
})
