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

package e2e

import (
	"context"
	"fmt"
	"time"

	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
	topologyclientset "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/clientset/versioned"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/test/e2e/framework"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils"
)

var _ = ginkgo.Describe("[RTE] Resource topology exporter", func() {
	var (
		initialized         bool
		nodeName            string
		namespace           string
		topologyClient      *topologyclientset.Clientset
		topologyUpdaterNode *v1.Node
		workerNodes         []v1.Node
	)

	f := framework.NewDefaultFramework("rte")

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

			ginkgo.By("getting the updated topology")
			// the object, hance the resource version must NOT change, so we can only sleep
			time.Sleep(30 * time.Second)
			ginkgo.By("checking the changes in the updated topology - expecting none")
			finalNodeTopo := getNodeTopology(topologyClient, topologyUpdaterNode.Name, namespace)

			initialAllocRes := allocatableResourceListFromNodeResourceTopology(initialNodeTopo)
			finalAllocRes := allocatableResourceListFromNodeResourceTopology(finalNodeTopo)
			if len(initialAllocRes) == 0 || len(finalAllocRes) == 0 {
				ginkgo.Fail(fmt.Sprintf("failed to find allocatable resources from node topology initial=%v final=%v", initialAllocRes, finalAllocRes))
			}
			zoneName, resName, isLess := lessAllocatableResources(initialAllocRes, finalAllocRes)
			framework.Logf("zone=%q resource=%q isLess=%v", zoneName, resName, isLess)
			gomega.Expect(isLess).To(gomega.BeFalse(), fmt.Sprintf("final allocatable resources not equal - initial=%v final=%v", initialAllocRes, finalAllocRes))
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

	})
})

func allocatableResourceListFromNodeResourceTopology(nodeTopo *v1alpha1.NodeResourceTopology) map[string]v1.ResourceList {
	allocRes := make(map[string]v1.ResourceList)
	for _, zone := range nodeTopo.Zones {
		if zone.Type != "Node" {
			continue
		}
		resList := make(v1.ResourceList)
		for _, res := range zone.Resources {
			resList[v1.ResourceName(res.Name)] = *resource.NewQuantity(int64(res.Allocatable.IntValue()), resource.DecimalSI)
		}
		if len(resList) == 0 {
			continue
		}
		allocRes[zone.Name] = resList
	}
	return allocRes
}

func lessAllocatableResources(expected, got map[string]v1.ResourceList) (string, string, bool) {
	if len(got) > len(expected) {
		return "", "", false
	}
	for expZoneName, expResList := range expected {
		gotResList, ok := got[expZoneName]
		if !ok {
			return expZoneName, "", false
		}
		if resName, ok := lessResourceList(expResList, gotResList); ok {
			return expZoneName, resName, true
		}
	}
	return "", "", false
}

func lessResourceList(expected, got v1.ResourceList) (string, bool) {
	if len(got) > len(expected) {
		return "", false
	}
	for expResName, expResQty := range expected {
		gotResQty, ok := got[expResName]
		if !ok {
			return string(expResName), false
		}
		framework.Logf("resource=%q expected=%v got=%v", expResName, expResQty, gotResQty)
		if gotResQty.Cmp(expResQty) < 1 {
			return string(expResName), true
		}
	}
	return "", false
}

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
