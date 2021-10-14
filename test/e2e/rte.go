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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	goversion "github.com/hashicorp/go-version"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/test/e2e/framework"

	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
	topologyclientset "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/clientset/versioned"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/nrtupdater"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/version"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils"
)

const (
	rteLabelName     = "resource-topology"
	rteContainerName = "resource-topology-exporter-container"
	defaultNamespace = "default"
)

var _ = ginkgo.Describe("[RTE][InfraConsuming] Resource topology exporter", func() {
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
			nodeName = utils.GetNodeName()
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
		ginkgo.It("it should react to pod changes using the smart poller", func() {
			nodes, err := utils.FilterNodesWithEnoughCores(workerNodes, "1000m")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			if len(nodes) < 1 {
				ginkgo.Skip("not enough allocatable cores for this test")
			}

			initialNodeTopo := getNodeTopology(topologyClient, topologyUpdaterNode.Name, namespace)
			ginkgo.By("creating a pod consuming the shared pool")
			sleeperPod := utils.MakeGuaranteedSleeperPod("1000m")

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
			}, 3*time.Second, 1*time.Second).Should(gomega.BeTrue(), "didn't get updated node topology info")
			ginkgo.By("checking the topology was updated for the right reason")

			gomega.Expect(finalNodeTopo.Annotations).ToNot(gomega.BeNil(), "missing annotations entirely")
			reason := finalNodeTopo.Annotations[nrtupdater.AnnotationRTEUpdate]
			gomega.Expect(reason).To(gomega.Equal(nrtupdater.RTEUpdateReactive), "update reason error: expected %q got %q", nrtupdater.RTEUpdateReactive, reason)
		})
	})

	ginkgo.Context("with the binary available", func() {
		ginkgo.It("it should show the correct version", func() {
			cmdline := []string{
				filepath.Join(utils.BinariesPath, "resource-topology-exporter"),
				"--version",
			}
			fmt.Fprintf(ginkgo.GinkgoWriter, "running: %v\n", cmdline)

			cmd := exec.Command(cmdline[0], cmdline[1:]...)
			cmd.Stderr = ginkgo.GinkgoWriter
			out, err := cmd.Output()
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			text := strings.TrimSpace(strings.Trim(string(out), version.ProgramName))
			_, err = goversion.NewVersion(text)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})
	})
})

func availableResourceListFromNodeResourceTopology(nodeTopo *v1alpha1.NodeResourceTopology) map[string]v1.ResourceList {
	availRes := make(map[string]v1.ResourceList)
	for _, zone := range nodeTopo.Zones {
		if zone.Type != "Node" {
			continue
		}
		resList := make(v1.ResourceList)
		for _, res := range zone.Resources {
			resList[v1.ResourceName(res.Name)] = res.Available
		}
		if len(resList) == 0 {
			continue
		}
		availRes[zone.Name] = resList
	}
	return availRes
}

func lessAvailableResources(expected, got map[string]v1.ResourceList) (string, string, bool) {
	zoneName, resName, cmp, ok := cmpAvailableResources(expected, got)
	if !ok {
		framework.Logf("-> cmp failed (not ok)")
		return "", "", false
	}
	if cmp < 0 {
		return zoneName, resName, true
	}
	framework.Logf("-> cmp failed (value=%d)", cmp)
	return "", "", false
}

func cmpAvailableResources(expected, got map[string]v1.ResourceList) (string, string, int, bool) {
	if len(got) != len(expected) {
		framework.Logf("-> expected=%v (len=%d) got=%v (len=%d)", expected, len(expected), got, len(got))
		return "", "", 0, false
	}
	for expZoneName, expResList := range expected {
		gotResList, ok := got[expZoneName]
		if !ok {
			return expZoneName, "", 0, false
		}
		if resName, cmp, ok := cmpResourceList(expResList, gotResList); !ok || cmp != 0 {
			return expZoneName, resName, cmp, ok
		}
	}
	return "", "", 0, true
}

func cmpResourceList(expected, got v1.ResourceList) (string, int, bool) {
	if len(got) != len(expected) {
		framework.Logf("-> expected=%v (len=%d) got=%v (len=%d)", expected, len(expected), got, len(got))
		return "", 0, false
	}
	for expResName, expResQty := range expected {
		gotResQty, ok := got[expResName]
		if !ok {
			return string(expResName), 0, false
		}
		if cmp := gotResQty.Cmp(expResQty); cmp != 0 {
			framework.Logf("-> resource=%q cmp=%d expected=%v got=%v", expResName, cmp, expResQty, gotResQty)
			return string(expResName), cmp, true
		}
	}
	return "", 0, true
}

func getNamespaceName() string {
	if nsName, ok := os.LookupEnv("E2E_NAMESPACE_NAME"); ok {
		return nsName
	}
	return defaultNamespace
}
