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
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/test/e2e/framework"
	k8se2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	admissionapi "k8s.io/pod-security-admission/api"

	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha2"
	topologyclientset "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/clientset/versioned"
	"github.com/k8stopologyawareschedwg/podfingerprint"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/k8sannotations"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/nrtupdater"

	e2etestns "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/namespace"
	e2enodes "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/nodes"
	e2enodetopology "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/nodetopology"
	e2epods "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/pods"
	e2ertepod "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/pods/rtepod"
	e2econsts "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/testconsts"
	e2etestenv "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/testenv"
)

const (
	updateIntervalExtraSafety = 10 * time.Second
)

var _ = ginkgo.Describe("[RTE][InfraConsuming] Resource topology exporter", func() {
	var (
		initialized         bool
		topologyClient      *topologyclientset.Clientset
		topologyUpdaterNode *corev1.Node
		workerNodes         []corev1.Node
	)

	f := framework.NewDefaultFramework("rte")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged
	f.SkipNamespaceCreation = true

	ginkgo.BeforeEach(func() {
		var err error

		err = e2etestns.Setup(f)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		if !initialized {
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

			initialized = true
		}
	})

	ginkgo.Context("with cluster configured", func() {
		ginkgo.It("[DEPRECATED][StateDirectories] it should react to pod changes using the smart poller", func() {
			nodes, err := e2enodes.FilterNodesWithEnoughCores(workerNodes, "1000m")
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			if len(nodes) < 1 {
				ginkgo.Skip("not enough allocatable cores for this test")
			}

			initialNodeTopo := e2enodetopology.GetNodeTopology(topologyClient, topologyUpdaterNode.Name)
			ginkgo.By("creating a pod consuming the shared pool")
			sleeperPod := e2epods.MakeGuaranteedSleeperPod("1000m")

			updateInterval, method, err := estimateUpdateInterval(*initialNodeTopo)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			klog.Infof("%s update interval: %s", method, updateInterval)

			// wait interval exactly multiple of the poll interval makes the test racier and less robust, so
			// add a little skew. We pick 1 second randomly, but the idea is that small (2, 3, 5) multipliers
			// should again not cause a total multiple of the poll interval.
			pollingInterval := updateInterval + time.Second

			stopChan := make(chan struct{})
			doneChan := make(chan struct{})
			started := false

			go func(cs clientset.Interface, podCli *k8se2epod.PodClient, refPod *corev1.Pod) {
				defer ginkgo.GinkgoRecover()

				<-stopChan

				pod := podCli.CreateSync(refPod)
				ginkgo.By("waiting for at least poll interval seconds with the test pod running...")
				time.Sleep(updateInterval * 3)
				e2epods.DeletePodSyncByName(cs, pod.Namespace, pod.Name)

				doneChan <- struct{}{}
			}(f.ClientSet, k8se2epod.NewPodClient(f), sleeperPod)

			ginkgo.By("getting the updated topology")
			var finalNodeTopo *v1alpha2.NodeResourceTopology
			gomega.Eventually(func() bool {
				if !started {
					stopChan <- struct{}{}
					started = true
				}

				finalNodeTopo, err = topologyClient.TopologyV1alpha2().NodeResourceTopologies().Get(context.TODO(), topologyUpdaterNode.Name, metav1.GetOptions{})
				if err != nil {
					klog.Infof("failed to get the node topology resource: %v", err)
					return false
				}
				if finalNodeTopo.ObjectMeta.ResourceVersion == initialNodeTopo.ObjectMeta.ResourceVersion {
					klog.Infof("resource %s not yet updated - resource version not bumped (old %v new %v)", topologyUpdaterNode.Name, initialNodeTopo.ObjectMeta.ResourceVersion, finalNodeTopo.ObjectMeta.ResourceVersion)
					return false
				}
				klog.Infof("resource %s updated! - resource version bumped (old %v new %v)", topologyUpdaterNode.Name, initialNodeTopo.ObjectMeta.ResourceVersion, finalNodeTopo.ObjectMeta.ResourceVersion)

				reason, ok := finalNodeTopo.Annotations[k8sannotations.RTEUpdate]
				if !ok {
					klog.Infof("resource %s missing annotation!", topologyUpdaterNode.Name)
					return false
				}
				klog.Infof("resource %s reason %v expected %v", topologyUpdaterNode.Name, reason, nrtupdater.RTEUpdateReactive)
				return reason == nrtupdater.RTEUpdateReactive
			}).WithTimeout(updateInterval*9).WithPolling(pollingInterval).Should(gomega.BeTrue(), "didn't get updated node topology info") // 5x timeout is a random "long enough" period
			ginkgo.By("checking the topology was updated for the right reason")

			<-doneChan

			gomega.Expect(finalNodeTopo.Annotations).ToNot(gomega.BeNil(), "missing annotations entirely")
			reason := finalNodeTopo.Annotations[k8sannotations.RTEUpdate]
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
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				rteNotifyFilePath, err := e2ertepod.FindNotificationFilePath(rtePod)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				execCommandInContainer(f, rtePod.Namespace, rtePod.Name, rteContainerName, "/bin/touch", rteNotifyFilePath)
				klog.Infof("notification triggered, exiting")
				doneChan <- struct{}{}
			}()

			ginkgo.By("getting the updated topology")
			var err error
			var finalNodeTopo *v1alpha2.NodeResourceTopology
			gomega.Eventually(func() bool {
				if !started {
					stopChan <- struct{}{}
					started = true
				}

				finalNodeTopo, err = topologyClient.TopologyV1alpha2().NodeResourceTopologies().Get(context.TODO(), topologyUpdaterNode.Name, metav1.GetOptions{})
				if err != nil {
					klog.Infof("failed to get the node topology resource: %v", err)
					return false
				}
				if finalNodeTopo.ObjectMeta.ResourceVersion == initialNodeTopo.ObjectMeta.ResourceVersion {
					klog.Infof("resource %s not yet updated - resource version not bumped", topologyUpdaterNode.Name)
					return false
				}

				klog.Infof("resource %s updated! - resource version bumped (old %v new %v)", topologyUpdaterNode.Name, initialNodeTopo.ObjectMeta.ResourceVersion, finalNodeTopo.ObjectMeta.ResourceVersion)

				reason, ok := finalNodeTopo.Annotations[k8sannotations.RTEUpdate]
				if !ok {
					klog.Infof("resource %s missing annotation!", topologyUpdaterNode.Name)
					return false
				}
				klog.Infof("resource %s reason %v expected %v", topologyUpdaterNode.Name, reason, nrtupdater.RTEUpdateReactive)
				return reason == nrtupdater.RTEUpdateReactive
			}).WithTimeout(31*time.Second).WithPolling(1*time.Second).Should(gomega.BeTrue(), "didn't get updated node topology info")
			ginkgo.By("checking the topology was updated for the right reason")

			<-doneChan

			gomega.Expect(finalNodeTopo.Annotations).ToNot(gomega.BeNil(), "missing annotations entirely")
			reason := finalNodeTopo.Annotations[k8sannotations.RTEUpdate]
			gomega.Expect(reason).To(gomega.Equal(nrtupdater.RTEUpdateReactive), "update reason error: expected %q got %q", nrtupdater.RTEUpdateReactive, reason)
		})
	})
	ginkgo.Context("with pod fingerprinting enabled", func() {
		ginkgo.It("[PodFingerprint] it should report the computation method in the attributes", func() {
			nrt := e2enodetopology.GetNodeTopology(topologyClient, topologyUpdaterNode.Name)
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
			prevNrt := e2enodetopology.GetNodeTopology(topologyClient, topologyUpdaterNode.Name)
			klog.Infof("Initial NRT: %q generation=%v resourceVersion=%v", prevNrt.Name, prevNrt.Generation, prevNrt.ResourceVersion)

			if _, ok := prevNrt.Annotations[podfingerprint.Annotation]; !ok {
				ginkgo.Skip("pod fingerprinting annotation not found - assuming disabled")
			}
			if _, ok := findAttribute(prevNrt.Attributes, podfingerprint.Attribute); !ok {
				ginkgo.Skip("pod fingerprinting attribute not found - assuming disabled")
			}

			dumpPods(f, topologyUpdaterNode.Name, "reference pods")

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

			currNrt := e2enodetopology.GetNodeTopology(topologyClient, topologyUpdaterNode.Name)
			klog.Infof("Control NRT: %q generation=%v resourceVersion=%v", prevNrt.Name, prevNrt.Generation, prevNrt.ResourceVersion)

			// note we don't test no pods have been added/deleted. This is because the suite is supposed to own the cluster while it runs
			// IOW, if we don't create/delete pods explicitely, noone else is supposed to do
			pfpStable := expectPodFingerprint(*prevNrt, "==", *currNrt)
			if !pfpStable {
				dumpPods(f, topologyUpdaterNode.Name, "after PFP mismatch")
				// ignore errors and carry on. We don't want to fail the test because of missing debug info.
				dumpRTELogs(f, topologyUpdaterNode.Name)

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
			prevNrt := e2enodetopology.GetNodeTopology(topologyClient, topologyUpdaterNode.Name)

			if _, ok := prevNrt.Annotations[podfingerprint.Annotation]; !ok {
				ginkgo.Skip("pod fingerprinting not found - assuming disabled")
			}
			if _, ok := findAttribute(prevNrt.Attributes, podfingerprint.Attribute); !ok {
				ginkgo.Skip("pod fingerprinting attribute not found - assuming disabled")
			}

			dumpPods(f, topologyUpdaterNode.Name, "reference pods")

			updateInterval, method, err := estimateUpdateInterval(*prevNrt)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			klog.Infof("%s update interval: %s", method, updateInterval)

			sleeperPod := e2epods.MakeGuaranteedSleeperPod("1000m")
			pod := k8se2epod.NewPodClient(f).CreateSync(sleeperPod)
			// (try to) delete the pod twice is no bother
			cs := f.ClientSet
			podNamespace, podName := pod.Namespace, pod.Name
			ginkgo.DeferCleanup(e2epods.DeletePodSyncByName, cs, podNamespace, podName)

			currNrt = getUpdatedNRT(topologyClient, topologyUpdaterNode.Name, *prevNrt, updateInterval)

			pfpChanged := expectPodFingerprint(*prevNrt, "!=", *currNrt)
			errMessage := "PFP did not change after pod creation"
			if !pfpChanged {
				dumpPods(f, topologyUpdaterNode.Name, errMessage)
			}
			gomega.Expect(pfpChanged).To(gomega.BeTrue(), errMessage)

			// since we need to delete the pod anyway, let's use this to run another check
			prevNrt = currNrt
			e2epods.DeletePodSyncByName(cs, podNamespace, podName)

			currNrt = getUpdatedNRT(topologyClient, topologyUpdaterNode.Name, *prevNrt, updateInterval)

			pfpChanged = expectPodFingerprint(*prevNrt, "!=", *currNrt)
			errMessage = "PFP did not change after pod deletion"
			if !pfpChanged {
				dumpPods(f, topologyUpdaterNode.Name, errMessage)
			}
			gomega.Expect(pfpChanged).To(gomega.BeTrue(), errMessage)
		})
	})
	ginkgo.Context("with refresh-node-resources enabled", func() {
		ginkgo.It("[NodeRefresh] should be able to detect devices", func() {
			gomega.Eventually(func() bool {
				nrt := e2enodetopology.GetNodeTopology(topologyClient, topologyUpdaterNode.Name)
				devName := e2etestenv.GetDeviceName()
				for _, zone := range nrt.Zones {
					for _, res := range zone.Resources {
						if res.Name == devName {
							return true
						}
					}
				}
				return false
			}, time.Second*30, time.Second*10).Should(gomega.BeTrue(), "device: %q was not found in NRT: %q", e2etestenv.GetDeviceName(), topologyUpdaterNode.Name)
		})

		ginkgo.It("[NodeRefresh] should log the refresh message", func() {
			rtePod, err := e2epods.GetPodOnNode(f, topologyUpdaterNode.Name, e2etestenv.GetNamespaceName(), e2etestenv.RTELabelName)
			framework.ExpectNoError(err)

			rteContainerName, err := e2ertepod.FindRTEContainerName(rtePod)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Eventually(func() bool {
				logs, err := e2epods.GetLogsForPod(f, rtePod.Namespace, rtePod.Name, rteContainerName)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				return strings.Contains(logs, "update node resources")
			}, time.Second*30, time.Second*10).Should(gomega.BeTrue(), "container: %q in pod: %q doesn't contains the refresh log message", rteContainerName, rtePod.Name)
		})
	})
})

func getUpdatedNRT(topologyClient *topologyclientset.Clientset, nodeName string, prevNrt v1alpha2.NodeResourceTopology, timeout time.Duration) *v1alpha2.NodeResourceTopology {
	var err error
	var currNrt *v1alpha2.NodeResourceTopology
	gomega.EventuallyWithOffset(1, func() bool {
		currNrt, err = topologyClient.TopologyV1alpha2().NodeResourceTopologies().Get(context.TODO(), nodeName, metav1.GetOptions{})
		if err != nil {
			klog.Infof("failed to get the node topology resource: %v", err)
			return false
		}
		if currNrt.ObjectMeta.ResourceVersion == prevNrt.ObjectMeta.ResourceVersion {
			klog.Infof("resource %s not yet updated - resource version not bumped", nodeName)
			return false
		}
		return true
	}, timeout+updateIntervalExtraSafety, 1*time.Second).Should(gomega.BeTrue(), "didn't get updated node topology info")
	return currNrt
}

func dumpPods(f *framework.Framework, nodeName, message string) {
	nodeSelector := fields.Set{
		"spec.nodeName": nodeName,
	}.AsSelector().String()

	pods, err := f.ClientSet.CoreV1().Pods(e2etestenv.GetNamespaceName()).List(context.TODO(), metav1.ListOptions{FieldSelector: nodeSelector})
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
	if err != nil {
		return fallbackInterval, "estimated", err
	}
	return updateInterval, "computed", nil
}

func execCommandInContainer(f *framework.Framework, namespace, podName, containerName string, cmd ...string) string {
	stdout, stderr, err := k8se2epod.ExecWithOptions(f, k8se2epod.ExecOptions{
		Command:            cmd,
		Namespace:          namespace,
		PodName:            podName,
		ContainerName:      containerName,
		Stdin:              nil,
		CaptureStdout:      true,
		CaptureStderr:      true,
		PreserveWhitespace: false,
	})
	klog.Infof("Exec stderr: %q", stderr)
	framework.ExpectNoError(err, "failed to execute command in namespace %v pod %v, container %v: %v", namespace, podName, containerName, err)
	return stdout
}

func dumpRTELogs(f *framework.Framework, nodeName string) error {
	rtePod, err := e2epods.GetPodOnNode(f, nodeName, e2etestenv.GetNamespaceName(), e2etestenv.RTELabelName)
	if err != nil {
		return err
	}

	rteContainerName, err := e2ertepod.FindRTEContainerName(rtePod)
	if err != nil {
		return err
	}

	logs, err := e2epods.GetLogsForPod(f, rtePod.Namespace, rtePod.Name, rteContainerName)
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
