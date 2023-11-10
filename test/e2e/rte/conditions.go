package rte

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/podreadiness"
	"github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/fixture"
	e2epods "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/pods"
	e2etestenv "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/testenv"
)

const crdName = "noderesourcetopologies.topology.node.k8s.io"

var _ = ginkgo.Describe("[RTE][Monitoring] conditions", func() {
	var (
		initialized bool
		namespace   string
		timeout     time.Duration
		crd         *apiextv1.CustomResourceDefinition
	)

	f := fixture.New()

	ginkgo.BeforeEach(func() {
		if !initialized {
			var err error

			namespace = e2etestenv.GetNamespaceName()

			timeout, err = time.ParseDuration(e2etestenv.GetPollInterval())
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			// wait interval exactly multiple of the poll interval makes the test racier and less robust, so
			// add a little skew. We pick 1 second randomly, but the idea is that small (2, 3, 5) multipliers
			// should again not cause a total multiple of the poll interval.
			timeout += 1 * time.Second

			// getting the CRD first, so we could recreate it later
			crd, err = f.ApiExt.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), crdName, metav1.GetOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			initialized = true
		}
	})

	// make sure to recreate the CRD even if the test failed
	ginkgo.AfterEach(func() {
		_, err := f.ApiExt.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), crdName, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			crd.ResourceVersion = ""
			_, err = f.ApiExt.ApiextensionsV1().CustomResourceDefinitions().Create(context.TODO(), crd, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		}
	})

	waitForPodCondition := func(podName string, conditionType podreadiness.RTEConditionType, expectedConditionStatus corev1.ConditionStatus) bool {
		pods, err := e2epods.GetPodsByLabel(f.K8SCli, namespace, fmt.Sprintf("name=%s", podName))
		if err != nil {
			return false
		}

		if len(pods) == 0 {
			return false
		}

		return cmpConditionsByTypeAndStatus(pods[0].Status.Conditions, conditionType, expectedConditionStatus)
	}

	ginkgo.Context("with NRT objects created", func() {
		ginkgo.It("[release] should have custom RTE conditions under the pod status", func() {
			gomega.Eventually(func() bool {
				return waitForPodCondition(e2etestenv.RTELabelName, podreadiness.PodresourcesFetched, corev1.ConditionTrue)
				// wait for twice the poll interval, so the conditions will have enough time to get updated
			}, 2*timeout, 1*time.Second).Should(gomega.BeTrue(), "pod contains wrong condition value")

			gomega.Eventually(func() bool {
				return waitForPodCondition(e2etestenv.RTELabelName, podreadiness.NodeTopologyUpdated, corev1.ConditionTrue)
				// wait for twice the poll interval, so the conditions will have enough time to get updated
			}, 2*timeout, 1*time.Second).Should(gomega.BeTrue(), "pod contains wrong condition value")
		})

		// EventChain means that the test can be flaky in some specific cases, for example deleted CRD can be re-installed
		// by third component
		ginkgo.It("[Disruptive][EventChain] should change the RTE conditions under the pod status accordingly", func() {
			ginkgo.By("deleting the crd")

			err := f.ApiExt.ApiextensionsV1().CustomResourceDefinitions().Delete(context.TODO(), crdName, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Eventually(func() bool {
				return waitForPodCondition(e2etestenv.RTELabelName, podreadiness.NodeTopologyUpdated, corev1.ConditionFalse)
				// wait for twice the poll interval, so the conditions will have enough time to get updated
			}, 2*timeout, 1*time.Second).Should(gomega.BeTrue(), "pod contains wrong condition value")

			ginkgo.By("recreating the crd")
			crd.ResourceVersion = ""
			_, err = f.ApiExt.ApiextensionsV1().CustomResourceDefinitions().Create(context.TODO(), crd, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Eventually(func() bool {
				return waitForPodCondition(e2etestenv.RTELabelName, podreadiness.NodeTopologyUpdated, corev1.ConditionFalse)
			}, 2*timeout, 1*time.Second).Should(gomega.BeTrue(), "pod contains wrong condition value")
		})
	})
})

func cmpConditionsByTypeAndStatus(podConds []corev1.PodCondition, conditionType podreadiness.RTEConditionType, status corev1.ConditionStatus) bool {
	for _, cond := range podConds {
		if cond.Type == corev1.PodConditionType(conditionType) && cond.Status == status {
			return true
		}
	}
	return false
}
