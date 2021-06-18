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

package e2e

import (
	"context"
	"time"

	topologyclientset "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/clientset/versioned"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	extclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kubeletconfig "k8s.io/kubernetes/pkg/kubelet/apis/config"
	"k8s.io/kubernetes/test/e2e/framework"
	e2ekubelet "k8s.io/kubernetes/test/e2e/framework/kubelet"
)

var _ = ginkgo.Describe("[RTE] Node topology updater", func() {
	var (
		topologyClient      *topologyclientset.Clientset
		topologyUpdaterNode *v1.Node
		kubeletConfig       *kubeletconfig.KubeletConfiguration
	)

	f := framework.NewDefaultFramework("rte")

	ginkgo.BeforeEach(func() {
		var err error

		if topologyClient == nil {
			topologyClient, err = topologyclientset.NewForConfig(f.ClientConfig())
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		}

		label := labels.SelectorFromSet(map[string]string{"name": "resource-topology"})
		pods, err := f.ClientSet.CoreV1().Pods(f.Namespace.Name).List(context.TODO(), metav1.ListOptions{LabelSelector: label.String()})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(pods.Items).ToNot(gomega.BeEmpty())

		topologyUpdaterNode, err = f.ClientSet.CoreV1().Nodes().Get(context.TODO(), pods.Items[0].Spec.NodeName, metav1.GetOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		kubeletConfig, err = e2ekubelet.GetCurrentKubeletConfig(topologyUpdaterNode.Name, "", true)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	ginkgo.Context("with cluster configured", func() {
		ginkgo.It("should fill the node resource topologies CR with the data", func() {
			gomega.Eventually(func() bool {
				// TODO: we should avoid to use hardcoded namespace name
				nodeTopology, err := topologyClient.TopologyV1alpha1().NodeResourceTopologies("default").Get(context.TODO(), topologyUpdaterNode.Name, metav1.GetOptions{})
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
