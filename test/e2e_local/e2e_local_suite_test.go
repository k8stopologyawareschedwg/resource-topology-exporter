// SPDX-License-Identifier: Apache-2.0

package e2e_local

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	_ "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/rte_local"
)

func TestE2ELocal(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2E Local Suite")
}
