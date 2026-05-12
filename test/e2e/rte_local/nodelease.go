/*
Copyright 2024 The Kubernetes Authors.

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

package rte_local

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/nodelease"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils"
)

var _ = ginkgo.Describe("[RTE][Local][NodeLease] Node-local lease", func() {
	var leaseDir string

	ginkgo.BeforeEach(func() {
		var err error
		leaseDir, err = os.MkdirTemp("", "rte-lease-e2e-*")
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	})

	ginkgo.AfterEach(func() {
		gomega.Expect(os.RemoveAll(leaseDir)).To(gomega.Succeed())
	})

	ginkgo.It("[release] should allow a single holder to acquire the lease", func() {
		leaseFile := filepath.Join(leaseDir, "lease")
		nl, err := nodelease.New(leaseFile)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		defer nl.Close()

		gomega.Expect(nl.TryLock()).To(gomega.BeTrue())
	})

	ginkgo.It("[release] should be idempotent when called multiple times", func() {
		leaseFile := filepath.Join(leaseDir, "lease")
		nl, err := nodelease.New(leaseFile)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		defer nl.Close()

		gomega.Expect(nl.TryLock()).To(gomega.BeTrue())
		gomega.Expect(nl.TryLock()).To(gomega.BeTrue())
	})

	ginkgo.It("[release] should prevent a second holder from acquiring the same lease", func() {
		leaseFile := filepath.Join(leaseDir, "lease")

		first, err := nodelease.New(leaseFile)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		defer first.Close()
		gomega.Expect(first.TryLock()).To(gomega.BeTrue())

		second, err := nodelease.New(leaseFile)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		defer second.Close()
		gomega.Expect(second.TryLock()).To(gomega.BeFalse())
	})

	ginkgo.It("[release] should release the lease on Close and allow another holder", func() {
		leaseFile := filepath.Join(leaseDir, "lease")

		first, err := nodelease.New(leaseFile)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		gomega.Expect(first.TryLock()).To(gomega.BeTrue())

		second, err := nodelease.New(leaseFile)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		defer second.Close()

		gomega.Expect(second.TryLock()).To(gomega.BeFalse())

		gomega.Expect(first.Close()).To(gomega.Succeed())
		gomega.Expect(second.TryLock()).To(gomega.BeTrue())
	})

	ginkgo.It("[release] should release the lease when the holding process is killed", func() {
		leaseFile := filepath.Join(leaseDir, "lease")

		helperBin := filepath.Join(utils.BinariesPath, "rte-lease-holder")
		child := exec.Command(helperBin, leaseFile)
		child.Stdout = ginkgo.GinkgoWriter
		child.Stderr = ginkgo.GinkgoWriter

		err := child.Start()
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		fmt.Fprintf(ginkgo.GinkgoWriter, "started lease holder process pid=%d\n", child.Process.Pid)

		// give the child time to acquire
		time.Sleep(500 * time.Millisecond)

		nl, err := nodelease.New(leaseFile)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		defer nl.Close()

		gomega.Expect(nl.TryLock()).To(gomega.BeFalse(), "should not acquire lease while child holds it")
		fmt.Fprintf(ginkgo.GinkgoWriter, "correctly failed to acquire lease (child holds it)\n")

		gomega.Expect(child.Process.Kill()).To(gomega.Succeed())
		child.Wait()
		fmt.Fprintf(ginkgo.GinkgoWriter, "child process killed\n")

		gomega.Expect(nl.TryLock()).To(gomega.BeTrue(), "should acquire lease after child is killed")
		fmt.Fprintf(ginkgo.GinkgoWriter, "acquired lease after child death\n")
	})
})
