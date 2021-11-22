package rte

import (
	"context"
	"fmt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
	v1apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/test/e2e/framework"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podreadiness"
	e2eclient "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/client"
	e2epods "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/pods"
	e2etestenv "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/testenv"
)

const crdName = "noderesourcetopologies.topology.node.k8s.io"

var _ = ginkgo.Describe("[RTE][Monitoring] conditions", func() {
	var (
		initialized bool
		namespace   string
		extClient   *clientset.Clientset
		timeout     time.Duration
		crd         *v1apiextensions.CustomResourceDefinition
	)

	f := framework.NewDefaultFramework("conditions")

	ginkgo.BeforeEach(func() {
		if !initialized {
			var err error

			namespace = e2etestenv.GetNamespaceName()

			timeout, err = time.ParseDuration(e2etestenv.GetPollInterval())
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			extClient, err = e2eclient.NewK8sExtFromFramework(f)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			// getting the CRD first, so we could recreate it later
			crd, err = extClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), crdName, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			initialized = true
		}
	})

	// make sure to recreate the CRD even if the test failed
	ginkgo.AfterEach(func() {
		_, err := extClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), crdName, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			crd.ResourceVersion = ""
			_, err = extClient.ApiextensionsV1().CustomResourceDefinitions().Create(context.TODO(), crd, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		}
	})

	ginkgo.Context("with NRT objects created", func() {
		ginkgo.It("should have custom RTE conditions under the pod status", func() {

			gomega.Eventually(func() bool {
				pods, err := e2epods.GetPodsByLabel(f, namespace, fmt.Sprintf("name=%s", e2etestenv.RTELabelName))
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(len(pods)).NotTo(gomega.BeZero())

				return cmpConditionsByTypeAndStatus(pods[0].Status.Conditions, podreadiness.PodresourcesFetched, v1.ConditionTrue)
				// wait for twice the poll interval, so the conditions will have enough time to get updated
			}, 2*timeout, 1*time.Second).Should(gomega.BeTrue(), "pod contains wrong condition value")

			gomega.Eventually(func() bool {
				pods, err := e2epods.GetPodsByLabel(f, namespace, fmt.Sprintf("name=%s", e2etestenv.RTELabelName))
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(len(pods)).NotTo(gomega.BeZero())

				return cmpConditionsByTypeAndStatus(pods[0].Status.Conditions, podreadiness.NodeTopologyUpdated, v1.ConditionTrue)
				// wait for twice the poll interval, so the conditions will have enough time to get updated
			}, 2*timeout, 1*time.Second).Should(gomega.BeTrue(), "pod contains wrong condition value")
		})

		ginkgo.It("should change the RTE conditions under the pod status accordingly", func() {
			ginkgo.By("deleting the crd")

			err := extClient.ApiextensionsV1().CustomResourceDefinitions().Delete(context.TODO(), crdName, metav1.DeleteOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gomega.Eventually(func() bool {
				pods, err := e2epods.GetPodsByLabel(f, namespace, fmt.Sprintf("name=%s", e2etestenv.RTELabelName))
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(len(pods)).NotTo(gomega.BeZero())

				return cmpConditionsByTypeAndStatus(pods[0].Status.Conditions, podreadiness.NodeTopologyUpdated, v1.ConditionFalse)
				// wait for twice the poll interval, so the conditions will have enough time to get updated
			}, 2*timeout, 1*time.Second).Should(gomega.BeTrue(), "pod contains wrong condition value")

			ginkgo.By("recreating the crd")
			crd.ResourceVersion = ""
			_, err = extClient.ApiextensionsV1().CustomResourceDefinitions().Create(context.TODO(), crd, metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gomega.Eventually(func() bool {
				pods, err := e2epods.GetPodsByLabel(f, namespace, fmt.Sprintf("name=%s", e2etestenv.RTELabelName))
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(len(pods)).NotTo(gomega.BeZero())

				return cmpConditionsByTypeAndStatus(pods[0].Status.Conditions, podreadiness.NodeTopologyUpdated, v1.ConditionFalse)
			}, 2*timeout, 1*time.Second).Should(gomega.BeTrue(), "pod contains wrong condition value")
		})
	})
})

func cmpConditionsByTypeAndStatus(podConds []v1.PodCondition, conditionType podreadiness.RTEConditionType, status v1.ConditionStatus) bool {
	for _, cond := range podConds {
		if cond.Type == v1.PodConditionType(conditionType) && cond.Status == status {
			return true
		}
	}
	return false
}
