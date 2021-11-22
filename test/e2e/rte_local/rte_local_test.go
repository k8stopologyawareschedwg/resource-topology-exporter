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
 * resource-topology-exporter specific tests, which require access
 * to the RTE binary and not to the cluster
 */

package rte_local

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	goversion "github.com/hashicorp/go-version"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/version"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils"
)

var _ = ginkgo.Describe("[RTE][Local] Resource topology exporter", func() {
	ginkgo.Context("with the binary available", func() {
		ginkgo.It("it should show the correct version", func() {
			cmdline := []string{
				filepath.Join(utils.BinariesPath, "resource-topology-exporter"),
				"--version",
			}
			fmt.Fprintf(ginkgo.GinkgoWriter, "running: %v\n", cmdline)

			cmd := exec.Command(cmdline[0], cmdline[1:]...)
			cmd.Stderr = ginkgo.GinkgoWriter
			out, err := cmd.Output()
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			text := strings.TrimSpace(strings.Trim(string(out), version.ProgramName))
			_, err = goversion.NewVersion(text)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})
	})
})
