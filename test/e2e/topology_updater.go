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
	"strings"
	"time"

	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
	topologyclientset "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/clientset/versioned"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeletconfig "k8s.io/kubernetes/pkg/kubelet/apis/config"
	"k8s.io/kubernetes/test/e2e/framework"
	e2ekubelet "k8s.io/kubernetes/test/e2e/framework/kubelet"
)

var _ = ginkgo.Describe("[TopologyUpdater][InfraConsuming] Node topology updater", func() {
	var (
		initialized         bool
		topologyClient      *topologyclientset.Clientset
		topologyUpdaterNode *v1.Node
		kubeletConfig       *kubeletconfig.KubeletConfiguration
	)

	f := framework.NewDefaultFramework("topology-updater")
	ns := getNamespaceName()

	ginkgo.BeforeEach(func() {
		var err error

		if !initialized {
			topologyClient, err = topologyclientset.NewForConfig(f.ClientConfig())
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			topologyUpdaterNode, err = f.ClientSet.CoreV1().Nodes().Get(context.TODO(), getNodeName(), metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			initialized = true
		}

		// intentionally get every single time
		kubeletConfig, err = e2ekubelet.GetCurrentKubeletConfig(topologyUpdaterNode.Name, "", true)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	ginkgo.Context("with cluster configured", func() {
		ginkgo.It("should fill the node resource topologies CR with the data", func() {
			nodeTopology := getNodeTopology(topologyClient, topologyUpdaterNode.Name, ns)
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
