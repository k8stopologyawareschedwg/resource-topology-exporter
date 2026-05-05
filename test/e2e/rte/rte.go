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

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/k8stopologyawareschedwg/numaplacement"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha2"
	topologyclientset "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/clientset/versioned"
	"github.com/k8stopologyawareschedwg/podfingerprint"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/k8sannotations"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/nrtupdater"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/fixture"
	e2enodes "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/nodes"
	e2enodetopology "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/nodetopology"
	e2epods "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/pods"
	e2ertepod "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/pods/rtepod"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/remoteexec"
	e2econsts "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/testconsts"
	e2etestenv "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/testenv"
)

const (
	updateIntervalExtraSafety = 10 * time.Second
)

var _ = ginkgo.Describe("[RTE][InfraConsuming] Resource topology exporter", func() {
	var (
		initialized         bool
		topologyUpdaterNode *corev1.Node
		workerNodes         []corev1.Node
	)

	f := fixture.New()

	ginkgo.BeforeEach(func() {
		var err error

		nsCleanup, err := f.CreateNamespace("rte")
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		ginkgo.DeferCleanup(nsCleanup)
		if !initialized {
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

			initialized = true
		}
	})

	ginkgo.Context("with cluster configured", func() {
		ginkgo.It("[NotificationFile] it should react to pod changes using the smart poller with notification file", func() {
			initialNodeTopo := e2enodetopology.GetNodeTopology(f.TopoCli, topologyUpdaterNode.Name)

			updateInterval, method, err := estimateUpdateInterval(*initialNodeTopo)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			baselineTimeout := 5*updateInterval + updateIntervalExtraSafety
			klog.Infof("%s update interval: %s (timeout %s)", method, updateInterval, baselineTimeout)

			ginkgo.By("waiting for a periodic update to find the optimal update window")
			var baselineNodeTopo *v1alpha2.NodeResourceTopology
			gomega.Eventually(func(g gomega.Gomega) {
				var err error
				baselineNodeTopo, err = f.TopoCli.TopologyV1alpha2().NodeResourceTopologies().Get(context.TODO(), topologyUpdaterNode.Name, metav1.GetOptions{})
				g.Expect(err).ToNot(gomega.HaveOccurred(), "failed to get the node topology resource")
				g.Expect(baselineNodeTopo.ObjectMeta.ResourceVersion).ToNot(gomega.Equal(initialNodeTopo.ObjectMeta.ResourceVersion), "resource %s not yet updated - resource version not bumped", topologyUpdaterNode.Name)
				klog.Infof("resource %s baseline update (resource version %v -> %v)", topologyUpdaterNode.Name, initialNodeTopo.ObjectMeta.ResourceVersion, baselineNodeTopo.ObjectMeta.ResourceVersion)
			}).WithTimeout(baselineTimeout).WithPolling(1*time.Second).Should(gomega.Succeed(), "didn't get baseline periodic update")

			ginkgo.By("triggering notification using the file")
			rtePod, err := e2epods.GetPodOnNode(f.K8SCli, topologyUpdaterNode.Name, e2etestenv.GetNamespaceName(), e2etestenv.RTELabelName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			rteContainerName, err := e2ertepod.FindRTEContainerName(rtePod)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			rteNotifyFilePath, err := e2ertepod.FindNotificationFilePath(f.Ctx, f.K8SCli, rtePod)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			cmd := []string{"/bin/touch", rteNotifyFilePath}
			_, _, err = remoteexec.CommandOnPod(f.Ctx, f.K8SCli, rtePod, rteContainerName, cmd...)
			gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed exec command %v on pod %q", cmd, client.ObjectKeyFromObject(rtePod).String())
			klog.Infof("notification triggered")

			ginkgo.By("waiting for reactive topology update")
			var finalNodeTopo *v1alpha2.NodeResourceTopology
			gomega.Eventually(func(g gomega.Gomega) {
				finalNodeTopo, err = f.TopoCli.TopologyV1alpha2().NodeResourceTopologies().Get(context.TODO(), topologyUpdaterNode.Name, metav1.GetOptions{})
				g.Expect(err).ToNot(gomega.HaveOccurred(), "failed to get the node topology resource")
				g.Expect(finalNodeTopo.ObjectMeta.ResourceVersion).ToNot(gomega.Equal(baselineNodeTopo.ObjectMeta.ResourceVersion), "resource %s not yet updated - resource version not bumped", topologyUpdaterNode.Name)

				klog.Infof("resource %s updated! - resource version bumped (old %v new %v)", topologyUpdaterNode.Name, baselineNodeTopo.ObjectMeta.ResourceVersion, finalNodeTopo.ObjectMeta.ResourceVersion)

				reason, ok := finalNodeTopo.Annotations[k8sannotations.RTEUpdate]
				g.Expect(ok).To(gomega.BeTrue(), "resource %s missing annotation!", topologyUpdaterNode.Name)
				g.Expect(reason).To(gomega.Equal(nrtupdater.RTEUpdateReactive), "resource %s reason %v expected %v", topologyUpdaterNode.Name, reason, nrtupdater.RTEUpdateReactive)
			}).WithTimeout(baselineTimeout).WithPolling(1*time.Second).Should(gomega.Succeed(), "didn't get updated node topology info")

			ginkgo.By("checking the topology was updated for the right reason")
			gomega.Expect(finalNodeTopo.Annotations).ToNot(gomega.BeNil(), "missing annotations entirely")
			reason := finalNodeTopo.Annotations[k8sannotations.RTEUpdate]
			gomega.Expect(reason).To(gomega.Equal(nrtupdater.RTEUpdateReactive), "update reason error: expected %q got %q", nrtupdater.RTEUpdateReactive, reason)
		})
	})

	ginkgo.Context("with pod fingerprinting enabled", func() {
		ginkgo.It("[PodFingerprint] it should report the computation method in the attributes", func() {
			nrt := e2enodetopology.GetNodeTopology(f.TopoCli, topologyUpdaterNode.Name)
			klog.Infof("Initial NRT: %q generation=%v resourceVersion=%v", nrt.Name, nrt.Generation, nrt.ResourceVersion)

			if _, ok := findAttribute(nrt.Attributes, podfingerprint.Attribute); !ok {
				ginkgo.Skip("pod fingerprinting attribute not found - assuming disabled")
			}
			meth, ok := findAttribute(nrt.Attributes, podfingerprint.AttributeMethod)
			gomega.Expect(ok).To(gomega.BeTrue(), "attribute %q missing, but PFP reported", podfingerprint.AttributeMethod)
			// note this is a subset of all the available methods declared in the podfingerprint package
			validMethods := []string{
				podfingerprint.MethodAll,
				podfingerprint.MethodWithExclusiveResources,
			}
			gomega.Expect(validMethods).Should(gomega.ContainElement(meth), "unsupported PFP computation method %q", meth)
		})

		ginkgo.It("[PodFingerprint] it should report stable value if the pods do not change", func() {
			prevNrt := e2enodetopology.GetNodeTopology(f.TopoCli, topologyUpdaterNode.Name)
			klog.Infof("Initial NRT: %q generation=%v resourceVersion=%v", prevNrt.Name, prevNrt.Generation, prevNrt.ResourceVersion)

			if _, ok := prevNrt.Annotations[podfingerprint.Annotation]; !ok {
				ginkgo.Skip("pod fingerprinting annotation not found - assuming disabled")
			}
			if _, ok := findAttribute(prevNrt.Attributes, podfingerprint.Attribute); !ok {
				ginkgo.Skip("pod fingerprinting attribute not found - assuming disabled")
			}

			dumpPods(f.K8SCli, topologyUpdaterNode.Name, "reference pods")

			updateInterval, method, err := estimateUpdateInterval(*prevNrt)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			klog.Infof("%s update interval: %s", method, updateInterval)

			// 3 timess is "long enough" - decided after quick tuning and try/error
			// if the object does not change, neither resourceVersion will. So we can just sleep.
			maxSteps := 3
			for step := 0; step < maxSteps; step++ {
				klog.Infof("waiting for %s: %d/%d", updateInterval, step+1, maxSteps)
				time.Sleep(updateInterval)
			}

			currNrt := e2enodetopology.GetNodeTopology(f.TopoCli, topologyUpdaterNode.Name)
			klog.Infof("Control NRT: %q generation=%v resourceVersion=%v", prevNrt.Name, prevNrt.Generation, prevNrt.ResourceVersion)

			// note we don't test no pods have been added/deleted. This is because the suite is supposed to own the cluster while it runs
			// IOW, if we don't create/delete pods explicitely, noone else is supposed to do
			pfpStable := expectPodFingerprint(*prevNrt, "==", *currNrt)
			if !pfpStable {
				dumpPods(f.K8SCli, topologyUpdaterNode.Name, "after PFP mismatch")
				// ignore errors and carry on. We don't want to fail the test because of missing debug info.
				dumpRTELogs(f.K8SCli, topologyUpdaterNode.Name)

			}
			gomega.Expect(pfpStable).To(gomega.BeTrue(), "PFP changed unexpectedly")
		})

		ginkgo.It("[release][PodFingerprint] it should report updated value if the set of running pods changes", func() {
			nodes, err := e2enodes.FilterNodesWithEnoughCores(workerNodes, "1000m")
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			if len(nodes) < 1 {
				ginkgo.Skip("not enough allocatable cores for this test")
			}

			var currNrt *v1alpha2.NodeResourceTopology
			prevNrt := e2enodetopology.GetNodeTopology(f.TopoCli, topologyUpdaterNode.Name)

			if _, ok := prevNrt.Annotations[podfingerprint.Annotation]; !ok {
				ginkgo.Skip("pod fingerprinting not found - assuming disabled")
			}
			if _, ok := findAttribute(prevNrt.Attributes, podfingerprint.Attribute); !ok {
				ginkgo.Skip("pod fingerprinting attribute not found - assuming disabled")
			}

			dumpPods(f.K8SCli, topologyUpdaterNode.Name, "reference pods")

			updateInterval, method, err := estimateUpdateInterval(*prevNrt)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			klog.Infof("%s update interval: %s", method, updateInterval)

			sleeperPod := e2epods.MakeGuaranteedSleeperPod("1000m")
			pod, err := e2epods.CreateSync(f, sleeperPod)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			// (try to) delete the pod twice is no bother
			podNamespace, podName := pod.Namespace, pod.Name
			ginkgo.DeferCleanup(e2epods.DeletePodSyncByName, f, podNamespace, podName)

			currNrt = getUpdatedNRT(f.TopoCli, topologyUpdaterNode.Name, *prevNrt, updateInterval)

			pfpChanged := expectPodFingerprint(*prevNrt, "!=", *currNrt)
			errMessage := "PFP did not change after pod creation"
			if !pfpChanged {
				dumpPods(f.K8SCli, topologyUpdaterNode.Name, errMessage)
			}
			gomega.Expect(pfpChanged).To(gomega.BeTrue(), errMessage)

			// since we need to delete the pod anyway, let's use this to run another check
			prevNrt = currNrt
			err = e2epods.DeletePodSyncByName(f, podNamespace, podName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			currNrt = getUpdatedNRT(f.TopoCli, topologyUpdaterNode.Name, *prevNrt, updateInterval)

			pfpChanged = expectPodFingerprint(*prevNrt, "!=", *currNrt)
			errMessage = "PFP did not change after pod deletion"
			if !pfpChanged {
				dumpPods(f.K8SCli, topologyUpdaterNode.Name, errMessage)
			}
			gomega.Expect(pfpChanged).To(gomega.BeTrue(), errMessage)
		})

		// Node-level numaplacement.AttributeMetadata is published when pod fingerprinting is enabled; it holds
		// PackMetadata() from encoded container NUMA affinities (see resourcemonitor Scan + encodeContainerAffinities).
		// It is not a per-zone field; on single-NUMA nodes the encoder short-circuits and the value may stay empty
		// or unchanged when pods change.
		ginkgo.Context("[numaplacement] node-level container affinity metadata", func() {
			ginkgo.It("should remain stable while workloads are unchanged", func() {
				prevNrt := e2enodetopology.GetNodeTopology(f.TopoCli, topologyUpdaterNode.Name)
				klog.Infof("Initial NRT: %q generation=%v resourceVersion=%v", prevNrt.Name, prevNrt.Generation, prevNrt.ResourceVersion)

				if _, ok := findAttribute(prevNrt.Attributes, podfingerprint.Attribute); !ok {
					ginkgo.Skip("pod fingerprinting attribute not found - assuming disabled")
				}
				metaBefore, ok := findAttribute(prevNrt.Attributes, numaplacement.AttributeMetadata)
				if !ok {
					ginkgo.Skip("numaplacement metadata attribute not found - RTE may not expose container NUMA affinity encoding")
				}

				dumpPods(f.K8SCli, topologyUpdaterNode.Name, "reference pods")

				updateInterval, method, err := estimateUpdateInterval(*prevNrt)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				klog.Infof("%s update interval: %s", method, updateInterval)

				maxSteps := 3
				for step := 0; step < maxSteps; step++ {
					klog.Infof("waiting for %s: %d/%d", updateInterval, step+1, maxSteps)
					time.Sleep(updateInterval)
				}

				// note we don't test no pods have been added/deleted. This is because the suite is supposed to own the cluster while it runs
				// IOW, if we don't create/delete pods explicitly, noone else is supposed to do
				currNrt := e2enodetopology.GetNodeTopology(f.TopoCli, topologyUpdaterNode.Name)
				klog.Infof("Current NRT: %q generation=%v resourceVersion=%v", currNrt.Name, currNrt.Generation, currNrt.ResourceVersion)

				metaAfter, ok := findAttribute(currNrt.Attributes, numaplacement.AttributeMetadata)
				gomega.Expect(ok).To(gomega.BeTrue(), "attribute %q missing after wait", numaplacement.AttributeMetadata)

				if metaBefore != metaAfter {
					dumpPods(f.K8SCli, topologyUpdaterNode.Name, "after numaplacement metadata mismatch")
					_ = dumpRTELogs(f.K8SCli, topologyUpdaterNode.Name)
				}

				gomega.Expect(metaAfter).To(gomega.Equal(metaBefore), "numaplacement metadata attribute changed unexpectedly")
			})

			ginkgo.It("should use the packed metadata prefix when the value is non-empty", func() {
				nrt := e2enodetopology.GetNodeTopology(f.TopoCli, topologyUpdaterNode.Name)
				if _, ok := findAttribute(nrt.Attributes, podfingerprint.Attribute); !ok {
					ginkgo.Skip("pod fingerprinting attribute not found - assuming disabled")
				}
				meta, ok := findAttribute(nrt.Attributes, numaplacement.AttributeMetadata)
				if !ok {
					ginkgo.Skip("numaplacement metadata attribute not found")
				}
				gomega.Expect(meta).ToNot(gomega.BeEmpty(), "numaplacement metadata is empty")
				gomega.Expect(meta).To(gomega.HavePrefix(numaplacement.Prefix + numaplacement.Version))
			})

			ginkgo.DescribeTable("numaplacement metadata on multi-NUMA nodes when pods are added and removed",
				func(qos corev1.PodQOSClass, resources corev1.ResourceList, expectPlacementMetadataChange bool) {
					for resName, resQty := range resources {
						nodes, err := e2enodes.FilterNodesWithEnoughResource(workerNodes, resName, resQty)
						gomega.Expect(err).ToNot(gomega.HaveOccurred())
						if len(nodes) < 1 {
							ginkgo.Skip("not enough allocatable resources for this test")
						}
					}

					prevNrt := e2enodetopology.GetNodeTopology(f.TopoCli, topologyUpdaterNode.Name)
					if _, ok := findAttribute(prevNrt.Attributes, podfingerprint.Attribute); !ok {
						ginkgo.Skip("pod fingerprinting attribute not found - assuming disabled")
					}
					metaBefore, ok := findAttribute(prevNrt.Attributes, numaplacement.AttributeMetadata)
					if !ok {
						ginkgo.Skip("numaplacement metadata attribute not found")
					}
					if len(prevNrt.Zones) < 2 {
						ginkgo.Skip("single NUMA zone in NRT: affinity encoding does not vary with pod set (encoder short-circuit)")
					}

					dumpPods(f.K8SCli, topologyUpdaterNode.Name, "reference pods")

					updateInterval, method, err := estimateUpdateInterval(*prevNrt)
					gomega.Expect(err).ToNot(gomega.HaveOccurred())
					klog.Infof("%s update interval: %s", method, updateInterval)

					sleeperPod := e2epods.MakeSleeperPod(qos, resources)
					pod, err := e2epods.CreateSync(f, sleeperPod)
					gomega.Expect(err).ToNot(gomega.HaveOccurred())
					podNamespace, podName := pod.Namespace, pod.Name
					ginkgo.DeferCleanup(e2epods.DeletePodSyncByName, f, podNamespace, podName)

					withPodNrt := getUpdatedNRT(f.TopoCli, topologyUpdaterNode.Name, *prevNrt, updateInterval)
					metaWithPod, ok := findAttribute(withPodNrt.Attributes, numaplacement.AttributeMetadata)
					gomega.Expect(ok).To(gomega.BeTrue(), "attribute %q missing after pod creation", numaplacement.AttributeMetadata)

					if expectPlacementMetadataChange {
						if metaWithPod == metaBefore {
							dumpPods(f.K8SCli, topologyUpdaterNode.Name, "metadata unchanged after pod creation")
							_ = dumpRTELogs(f.K8SCli, topologyUpdaterNode.Name)
						}
						gomega.Expect(metaWithPod).ToNot(gomega.Equal(metaBefore), "numaplacement metadata did not change after a workload with exclusive CPU placement was scheduled")
					} else {
						if metaWithPod != metaBefore {
							dumpPods(f.K8SCli, topologyUpdaterNode.Name, "metadata changed unexpectedly for workload without exclusive CPU")
							_ = dumpRTELogs(f.K8SCli, topologyUpdaterNode.Name)
						}
						gomega.Expect(metaWithPod).To(gomega.Equal(metaBefore), "numaplacement metadata should stay unchanged for workloads that do not get exclusive CPUs in the pod-resources API")
					}

					err = e2epods.DeletePodSyncByName(f, podNamespace, podName)
					gomega.Expect(err).ToNot(gomega.HaveOccurred())

					afterDeleteNrt := getUpdatedNRT(f.TopoCli, topologyUpdaterNode.Name, *withPodNrt, updateInterval)
					metaAfter, ok := findAttribute(afterDeleteNrt.Attributes, numaplacement.AttributeMetadata)
					gomega.Expect(ok).To(gomega.BeTrue())
					if metaAfter != metaBefore {
						dumpPods(f.K8SCli, topologyUpdaterNode.Name, "metadata mismatch after pod deletion")
						_ = dumpRTELogs(f.K8SCli, topologyUpdaterNode.Name)
					}
					gomega.Expect(metaAfter).To(gomega.Equal(metaBefore), "numaplacement metadata should return to baseline after pod removal")
				},
				ginkgo.Entry("guaranteed pod", corev1.PodQOSGuaranteed, corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1000m"),
					corev1.ResourceMemory: resource.MustParse("250Mi"),
				}, true),
				ginkgo.Entry("burstable pod - nothing exclusive", corev1.PodQOSBurstable, corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1000m"),
					corev1.ResourceMemory: resource.MustParse("250Mi"),
				}, false),
				ginkgo.Entry("burstable pod - exclusive device", corev1.PodQOSBurstable, corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("1000m"),
					corev1.ResourceName(e2etestenv.GetDeviceName()): resource.MustParse("1"),
				}, true),
				ginkgo.Entry("best effort pod - nothing exclusive", corev1.PodQOSBestEffort,
					corev1.ResourceList{}, false),
				ginkgo.Entry("best effort pod - exclusive device", corev1.PodQOSBestEffort,
					corev1.ResourceList{
						corev1.ResourceName(e2etestenv.GetDeviceName()): resource.MustParse("1"),
					}, true),
			)
		})
	})
	ginkgo.Context("with refresh-node-resources enabled", func() {
		ginkgo.It("[NodeRefresh] should be able to detect devices", func() {
			gomega.Eventually(func(g gomega.Gomega) {
				nrt := e2enodetopology.GetNodeTopology(f.TopoCli, topologyUpdaterNode.Name)
				devName := e2etestenv.GetDeviceName()
				found := false
				for _, zone := range nrt.Zones {
					for _, res := range zone.Resources {
						if res.Name == devName {
							found = true
						}
					}
				}
				g.Expect(found).To(gomega.BeTrue(), "device: %q was not found in NRT: %q", devName, topologyUpdaterNode.Name)
			}).WithTimeout(30 * time.Second).WithPolling(10 * time.Second).Should(gomega.Succeed())
		})

		ginkgo.It("[NodeRefresh] should log the refresh message", func() {
			rtePod, err := e2epods.GetPodOnNode(f.K8SCli, topologyUpdaterNode.Name, e2etestenv.GetNamespaceName(), e2etestenv.RTELabelName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			rteContainerName, err := e2ertepod.FindRTEContainerName(rtePod)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Eventually(func(g gomega.Gomega) {
				logs, err := e2epods.GetLogsForPod(f.K8SCli, rtePod.Namespace, rtePod.Name, rteContainerName)
				g.Expect(err).ToNot(gomega.HaveOccurred())

				g.Expect(logs).To(gomega.Or(
					gomega.ContainSubstring("tracking node resources"),
					gomega.ContainSubstring("update node resources"),
				), "container: %q in pod: %q doesn't contain the refresh log message", rteContainerName, rtePod.Name)
			}).WithTimeout(42 * time.Second).WithPolling(5 * time.Second).Should(gomega.Succeed())
		})
	})
})

func getUpdatedNRT(topologyClient *topologyclientset.Clientset, nodeName string, prevNrt v1alpha2.NodeResourceTopology, timeout time.Duration) *v1alpha2.NodeResourceTopology {
	ginkgo.GinkgoHelper()
	effectiveTimeout := timeout + updateIntervalExtraSafety
	klog.Infof("waiting for NRT %q update: timeout %v (base %v + safety %v) prev resourceVersion=%v", nodeName, effectiveTimeout, timeout, updateIntervalExtraSafety, prevNrt.ObjectMeta.ResourceVersion)
	var err error
	var currNrt *v1alpha2.NodeResourceTopology
	gomega.Eventually(func(g gomega.Gomega) {
		currNrt, err = topologyClient.TopologyV1alpha2().NodeResourceTopologies().Get(context.TODO(), nodeName, metav1.GetOptions{})
		g.Expect(err).ToNot(gomega.HaveOccurred(), "failed to get the node topology resource")
		g.Expect(currNrt.ObjectMeta.ResourceVersion).ToNot(gomega.Equal(prevNrt.ObjectMeta.ResourceVersion), "resource %s not yet updated - resource version not bumped", nodeName)
	}).WithTimeout(effectiveTimeout).WithPolling(1*time.Second).Should(gomega.Succeed(), "didn't get updated node topology info")
	return currNrt
}

func dumpPods(cs clientset.Interface, nodeName, message string) {
	nodeSelector := fields.Set{
		"spec.nodeName": nodeName,
	}.AsSelector().String()

	pods, err := cs.CoreV1().Pods(e2etestenv.GetNamespaceName()).List(context.TODO(), metav1.ListOptions{FieldSelector: nodeSelector})
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	klog.Infof("BEGIN pods running on %q: %s", nodeName, message)
	for _, pod := range pods.Items {
		klog.Infof("%s %s/%s status=%s (%s %s)", nodeName, pod.Namespace, pod.Name, pod.Status.Phase, pod.Status.Message, pod.Status.Reason)
	}
	klog.Infof("END pods running on %q: %s", nodeName, message)
}

func expectPodFingerprint(nrt1 v1alpha2.NodeResourceTopology, mode string, nrt2 v1alpha2.NodeResourceTopology) bool {
	pfp1, ok1 := extractPFP(nrt1)
	if !ok1 {
		return false
	}

	pfp2, ok2 := extractPFP(nrt2)
	if !ok2 {
		return false
	}

	switch mode {
	case "==":
		return expectEqualPFPs(nrt1.Name, pfp1, nrt2.Name, pfp2)
	case "!=":
		return expectDifferentPFPs(nrt1.Name, pfp1, nrt2.Name, pfp2)
	default:
		klog.Infof("unsupported comparison mode %q", mode)
		return false
	}
}

func extractPFP(nrt v1alpha2.NodeResourceTopology) (string, bool) {
	pfpAnn, okAnn := nrt.Annotations[podfingerprint.Annotation]
	if !okAnn {
		klog.Infof("cannot find pod fingerprint annotation in NRT %q", nrt.Name)
		return "", false
	}
	pfpAttr, okAttr := findAttribute(nrt.Attributes, podfingerprint.Attribute)
	if !okAttr {
		klog.Infof("cannot find pod fingerprint attribute in NRT %q", nrt.Name)
		return "", false
	}
	if pfpAnn != pfpAttr {
		klog.Infof("PFP mismatch in %q  annotation=%q attribute=%q", nrt.Name, pfpAnn, pfpAttr)
		return "", false
	}
	return pfpAttr, true
}

func expectEqualPFPs(name1, pfp1, name2, pfp2 string) bool {
	if pfp1 != pfp2 {
		klog.Infof("fingerprint mismatch NRT %q PFP %q vs NRT %q PFP %q", name1, pfp1, name2, pfp2)
		return false
	}
	return true
}

func expectDifferentPFPs(name1, pfp1, name2, pfp2 string) bool {
	if pfp1 == pfp2 {
		klog.Infof("fingerprint equality NRT %q PFP %q vs NRT %q PFP %q", name2, pfp1, name2, pfp2)
		return false
	}
	return true
}

func estimateUpdateInterval(nrt v1alpha2.NodeResourceTopology) (time.Duration, string, error) {
	fallbackInterval, err := time.ParseDuration(e2etestenv.GetPollInterval())
	if err != nil {
		return fallbackInterval, "estimated", err
	}
	klog.Infof("Annotations for %q: %#v", nrt.Name, nrt.Annotations)
	updateIntervalAnn, ok := nrt.Annotations[k8sannotations.UpdateInterval]
	if !ok {
		// no annotation, we need to guess
		return fallbackInterval, "estimated", nil
	}
	updateInterval, err := time.ParseDuration(updateIntervalAnn)
	if err != nil || updateInterval <= 0 {
		klog.Warningf("annotation %q has unusable value %q (parsed=%v err=%v), falling back to %v", k8sannotations.UpdateInterval, updateIntervalAnn, updateInterval, err, fallbackInterval)
		return fallbackInterval, "estimated", err
	}
	return updateInterval, "computed", nil
}

func dumpRTELogs(cs clientset.Interface, nodeName string) error {
	rtePod, err := e2epods.GetPodOnNode(cs, nodeName, e2etestenv.GetNamespaceName(), e2etestenv.RTELabelName)
	if err != nil {
		return err
	}

	rteContainerName, err := e2ertepod.FindRTEContainerName(rtePod)
	if err != nil {
		return err
	}

	logs, err := e2epods.GetLogsForPod(cs, rtePod.Namespace, rtePod.Name, rteContainerName)
	if err != nil {
		return err
	}

	klog.Infof("RTE logs:\n%s", logs)
	return nil
}

func findAttribute(attrs v1alpha2.AttributeList, name string) (string, bool) {
	for _, attr := range attrs {
		if attr.Name == name {
			return attr.Value, true
		}
	}
	return "", false
}
