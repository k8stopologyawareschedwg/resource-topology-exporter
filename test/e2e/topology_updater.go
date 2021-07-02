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
	"time"

	topologyclientset "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/clientset/versioned"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeletconfig "k8s.io/kubernetes/pkg/kubelet/apis/config"
	"k8s.io/kubernetes/test/e2e/framework"
	e2ekubelet "k8s.io/kubernetes/test/e2e/framework/kubelet"
)

var _ = ginkgo.Describe("[RTE] Node topology updater", func() {
	var (
		inited              bool
		topologyClient      *topologyclientset.Clientset
		topologyUpdaterNode *v1.Node
		kubeletConfig       *kubeletconfig.KubeletConfiguration
	)

	f := framework.NewDefaultFramework("topology-updater")
	ns := getNamespaceName()

	ginkgo.BeforeEach(func() {
		var err error

		if !inited {
			topologyClient, err = topologyclientset.NewForConfig(f.ClientConfig())
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			topologyUpdaterNode, err = f.ClientSet.CoreV1().Nodes().Get(context.TODO(), getNodeName(), metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			inited = true
		}

		// intentionally get every single time
		kubeletConfig, err = e2ekubelet.GetCurrentKubeletConfig(topologyUpdaterNode.Name, "", true)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	ginkgo.Context("with cluster configured", func() {
		ginkgo.It("should fill the node resource topologies CR with the data", func() {
			gomega.Eventually(func() bool {
				nodeTopology, err := topologyClient.TopologyV1alpha1().NodeResourceTopologies(ns).Get(context.TODO(), topologyUpdaterNode.Name, metav1.GetOptions{})
				if err != nil {
					framework.Logf("failed to get the node topology resource: %v", err)
					return false
				}

				if nodeTopology == nil || len(nodeTopology.TopologyPolicies) == 0 {
					framework.Logf("failed to get topology policy from the node topology resource")
					return false
				}

				if nodeTopology.TopologyPolicies[0] != (*kubeletConfig).TopologyManagerPolicy {
					return false
				}

				// TODO: add more checks like checking distances, NUMA node and allocated CPUs

				return true
			}, time.Minute, 5*time.Second).Should(gomega.BeTrue())
		})
	})
})
