package e2e

import (
	"context"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/test/e2e/framework"
)

var _ = ginkgo.Describe("[RTE] metrics", func() {
	var (
		initialized bool
		rtePod      *v1.Pod
	)

	f := framework.NewDefaultFramework("metrics")

	ginkgo.BeforeEach(func() {
		if !initialized {
			pods, err := f.ClientSet.CoreV1().Pods(defaultNamespace).List(context.TODO(), metav1.ListOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			for _, pod := range pods.Items {
				if val, ok := pod.Labels["name"]; ok && val == rteLabelName {
					rtePod = &pod
				}
			}
			gomega.Expect(rtePod).ToNot(gomega.BeNil())
			initialized = true
		}
	})

	ginkgo.Context("With prometheus endpoint configured", func() {
		ginkgo.It("should have some metrics exported", func() {
			stdout, stderr, err := f.ExecWithOptions(framework.ExecOptions{
				Command:            []string{"curl", "http://127.0.0.1:2112/metrics"},
				Namespace:          getNamespaceName(),
				PodName:            rtePod.Name,
				ContainerName:      rteContainerName,
				Stdin:              nil,
				CaptureStdout:      true,
				CaptureStderr:      true,
				PreserveWhitespace: false,
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "%s", stderr)
			gomega.Expect(stdout).To(gomega.ContainSubstring("operation_delay"))
		})
	})
})
