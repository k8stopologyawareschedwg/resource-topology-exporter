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

package e2e

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
	topologyclientset "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/clientset/versioned"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeletconfig "k8s.io/kubernetes/pkg/kubelet/apis/config"
	"k8s.io/kubernetes/test/e2e/framework"
	e2ekubelet "k8s.io/kubernetes/test/e2e/framework/kubelet"
)

var _ = ginkgo.Describe("[TopologyUpdater][InfraConsuming] Node topology updater", func() {
	var (
		initialized         bool
		nodeName            string
		namespace           string
		kubeletConfig       *kubeletconfig.KubeletConfiguration
		topologyClient      *topologyclientset.Clientset
		topologyUpdaterNode *v1.Node
		workerNodes         []v1.Node
	)

	f := framework.NewDefaultFramework("topology-updater")

	ginkgo.BeforeEach(func() {
		var err error

		if !initialized {
			nodeName = getNodeName()
			namespace = getNamespaceName()

			topologyClient, err = topologyclientset.NewForConfig(f.ClientConfig())
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			topologyUpdaterNode, err = f.ClientSet.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			workerNodes, err = utils.GetWorkerNodes(f)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			initialized = true
		}

		// intentionally done once
		kubeletConfig, err = e2ekubelet.GetCurrentKubeletConfig(topologyUpdaterNode.Name, "", true)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	ginkgo.Context("with cluster configured", func() {
		ginkgo.It("it should not account for any cpus if a container doesn't request exclusive cpus", func() {
			ginkgo.By("getting the initial topology information")
			initialNodeTopo := getNodeTopology(topologyClient, topologyUpdaterNode.Name, namespace)
			ginkgo.By("creating a pod consuming the shared pool")
			sleeperPod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sleeper-be-pod",
				},
				Spec: v1.PodSpec{
					RestartPolicy: v1.RestartPolicyNever,
					Containers: []v1.Container{
						v1.Container{
							Name:  "sleeper-be-cnt",
							Image: utils.CentosImage,
							// 1 hour (or >= 1h in general) is "forever" for our purposes
							Command: []string{"/bin/sleep", "1h"},
						},
					},
				},
			}

			podMap := make(map[string]*v1.Pod)
			pod := f.PodClient().CreateSync(sleeperPod)
			podMap[pod.Name] = pod
			defer utils.DeletePodsAsync(f, podMap)

			cooldown := 30 * time.Second
			ginkgo.By(fmt.Sprintf("getting the updated topology - sleeping for %v", cooldown))
			// the object, hance the resource version must NOT change, so we can only sleep
			time.Sleep(cooldown)
			ginkgo.By("checking the changes in the updated topology - expecting none")
			finalNodeTopo := getNodeTopology(topologyClient, topologyUpdaterNode.Name, namespace)

			initialAllocRes := allocatableResourceListFromNodeResourceTopology(initialNodeTopo)
			finalAllocRes := allocatableResourceListFromNodeResourceTopology(finalNodeTopo)
			if len(initialAllocRes) == 0 || len(finalAllocRes) == 0 {
				ginkgo.Fail(fmt.Sprintf("failed to find allocatable resources from node topology initial=%v final=%v", initialAllocRes, finalAllocRes))
			}
			zoneName, resName, cmp, ok := cmpAllocatableResources(initialAllocRes, finalAllocRes)
			framework.Logf("zone=%q resource=%q cmp=%v ok=%v", zoneName, resName, cmp, ok)
			if !ok {
				ginkgo.Fail(fmt.Sprintf("failed to compare allocatable resources from node topology initial=%v final=%v", initialAllocRes, finalAllocRes))
			}
			// this is actually a workaround. The proper solution is to wait for ALL the container requesting exclusive resources to be gone
			// -- so we are actually sure the accounting is correct. But we didn't found yet a generic way to do that on remote worker nodes.
			isGreaterEqual := (cmp >= 0)
			gomega.Expect(isGreaterEqual).To(gomega.BeTrue(), fmt.Sprintf("final allocatable resources not restored - cmp=%d initial=%v final=%v", cmp, initialAllocRes, finalAllocRes))
		})

		ginkgo.It("it should account for containers requesting exclusive cpus", func() {
			nodes, err := utils.FilterNodesWithEnoughCores(workerNodes, "1000m")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			if len(nodes) < 1 {
				ginkgo.Skip("not enough allocatable cores for this test")
			}

			ginkgo.By("getting the initial topology information")
			initialNodeTopo := getNodeTopology(topologyClient, topologyUpdaterNode.Name, namespace)
			ginkgo.By("creating a pod consuming the shared pool")
			sleeperPod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sleeper-gu-pod",
				},
				Spec: v1.PodSpec{
					RestartPolicy: v1.RestartPolicyNever,
					Containers: []v1.Container{
						v1.Container{
							Name:  "sleeper-gu-cnt",
							Image: utils.CentosImage,
							// 1 hour (or >= 1h in general) is "forever" for our purposes
							Command: []string{"/bin/sleep", "1h"},
							Resources: v1.ResourceRequirements{
								Limits: v1.ResourceList{
									// we use 1 core because that's the minimal meaningful quantity
									v1.ResourceName(v1.ResourceCPU): resource.MustParse("1000m"),
									// any random reasonable amount is fine
									v1.ResourceName(v1.ResourceMemory): resource.MustParse("100Mi"),
								},
							},
						},
					},
				},
			}

			podMap := make(map[string]*v1.Pod)
			pod := f.PodClient().CreateSync(sleeperPod)
			podMap[pod.Name] = pod
			defer utils.DeletePodsAsync(f, podMap)

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
			ginkgo.By("checking the changes in the updated topology")

			initialAllocRes := allocatableResourceListFromNodeResourceTopology(initialNodeTopo)
			finalAllocRes := allocatableResourceListFromNodeResourceTopology(finalNodeTopo)
			if len(initialAllocRes) == 0 || len(finalAllocRes) == 0 {
				ginkgo.Fail(fmt.Sprintf("failed to find allocatable resources from node topology initial=%v final=%v", initialAllocRes, finalAllocRes))
			}
			zoneName, resName, isLess := lessAllocatableResources(initialAllocRes, finalAllocRes)
			framework.Logf("zone=%q resource=%q isLess=%v", zoneName, resName, isLess)
			gomega.Expect(isLess).To(gomega.BeTrue(), fmt.Sprintf("final allocatable resources not decreased - initial=%v final=%v", initialAllocRes, finalAllocRes))
		})

		ginkgo.It("should fill the node resource topologies CR with the data", func() {
			nodeTopology := getNodeTopology(topologyClient, topologyUpdaterNode.Name, namespace)
			isValid := isValidNodeTopology(nodeTopology, kubeletConfig)
			gomega.Expect(isValid).To(gomega.BeTrue(), "received invalid topology: %v", nodeTopology)
		})
	})
})

func getNodeTopology(topologyClient *topologyclientset.Clientset, nodeName, namespace string) *v1alpha1.NodeResourceTopology {
	var nodeTopology *v1alpha1.NodeResourceTopology
	var err error
	gomega.EventuallyWithOffset(1, func() bool {
		nodeTopology, err = topologyClient.TopologyV1alpha1().NodeResourceTopologies(namespace).Get(context.TODO(), nodeName, metav1.GetOptions{})
		if err != nil {
			framework.Logf("failed to get the node topology resource: %v", err)
			return false
		}
		return true
	}, time.Minute, 5*time.Second).Should(gomega.BeTrue())
	return nodeTopology
}

func isValidNodeTopology(nodeTopology *v1alpha1.NodeResourceTopology, kubeletConfig *kubeletconfig.KubeletConfiguration) bool {
	if nodeTopology == nil || len(nodeTopology.TopologyPolicies) == 0 {
		framework.Logf("failed to get topology policy from the node topology resource")
		return false
	}

	if nodeTopology.TopologyPolicies[0] != (*kubeletConfig).TopologyManagerPolicy {
		return false
	}

	if nodeTopology.Zones == nil || len(nodeTopology.Zones) == 0 {
		framework.Logf("failed to get topology zones from the node topology resource")
		return false
	}

	foundNodes := 0
	for _, zone := range nodeTopology.Zones {
		// TODO constant not in the APIs
		if !strings.HasPrefix(strings.ToUpper(zone.Type), "NODE") {
			continue
		}
		foundNodes++

		if !isValidCostList(zone.Name, zone.Costs) {
			return false
		}

		if !isValidResourceList(zone.Name, zone.Resources) {
			return false
		}
	}
	return foundNodes > 0
}

func isValidCostList(zoneName string, costs v1alpha1.CostList) bool {
	if len(costs) == 0 {
		framework.Logf("failed to get topology costs for zone %q from the node topology resource", zoneName)
		return false
	}

	// TODO cross-validate zone names
	for _, cost := range costs {
		if cost.Name == "" || cost.Value < 0 {
			framework.Logf("malformed cost %v for zone %q", cost, zoneName)
		}
	}
	return true
}

func isValidResourceList(zoneName string, resources v1alpha1.ResourceInfoList) bool {
	if len(resources) == 0 {
		framework.Logf("failed to get topology resources for zone %q from the node topology resource", zoneName)
		return false
	}
	foundCpu := false
	for _, resource := range resources {
		// TODO constant not in the APIs
		if strings.ToUpper(resource.Name) == "CPU" {
			foundCpu = true
		}
		allocatable := resource.Allocatable.IntValue()
		capacity := resource.Capacity.IntValue()
		if (allocatable < 0 || capacity < 0) || (capacity < allocatable) {
			framework.Logf("malformed resource %v for zone %q", resource, zoneName)
			return false
		}
	}
	return foundCpu
}
