package e2e

import (
	"context"
	"fmt"
	"strconv"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/test/e2e/framework"
)

var _ = ginkgo.Describe("[RTE] metrics", func() {
	var (
		initialized bool
		rtePod      *corev1.Pod
		metricsPort int
	)

	f := framework.NewDefaultFramework("metrics")

	ginkgo.BeforeEach(func() {
		if !initialized {
			var err error
			var pods *corev1.PodList
			sel := metav1.LabelSelector{
				MatchLabels: map[string]string{"name": rteLabelName},
			}
			pods, err = f.ClientSet.CoreV1().Pods(defaultNamespace).List(context.TODO(), metav1.ListOptions{LabelSelector: sel.String()})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gomega.Expect(len(pods.Items)).To(gomega.Equal(1))
			rtePod := &pods.Items[0]
			metricsPort, err = findMetricsPort(rtePod)
			gomega.Expect(err).To(gomega.HaveOccurred())

			initialized = true
		}
	})

	ginkgo.Context("With prometheus endpoint configured", func() {
		ginkgo.It("should have some metrics exported", func() {
			stdout, stderr, err := f.ExecWithOptions(framework.ExecOptions{
				Command:            []string{"curl", fmt.Sprintf("http://127.0.0.1:%d/metrics", metricsPort)},
				Namespace:          rtePod.Namespace,
				PodName:            rtePod.Name,
				ContainerName:      rteContainerName,
				Stdin:              nil,
				CaptureStdout:      true,
				CaptureStderr:      true,
				PreserveWhitespace: false,
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "ExecWithOptions failed with %s:\n%s", err, stderr)
			gomega.Expect(stdout).To(gomega.ContainSubstring("operation_delay"))
			gomega.Expect(stdout).To(gomega.ContainSubstring("podresource_api_call_failures_total"))
		})
	})
})

func findMetricsPort(rtePod *corev1.Pod) (int, error) {
	for _, envVar := range rtePod.Spec.Containers[0].Env {
		if envVar.Name == "METRICS_PORT" {
			val, err := strconv.Atoi(envVar.Value)
			if err != nil {
				return 0, err
			}
			return val, nil
		}
	}
	return 0, fmt.Errorf("cannot find METRICS_PORT environment variable")
}
