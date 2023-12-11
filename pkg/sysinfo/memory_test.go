/*
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

package sysinfo

import (
	"os"
	"path/filepath"
	"testing"

	v1 "k8s.io/api/core/v1"
)

const testMeminfo = `Node 0 MemTotal:       32718644 kB
Node 0 MemFree:         2915988 kB
Node 0 MemUsed:        29802656 kB
Node 0 Active:         19631832 kB
Node 0 Inactive:        8089096 kB
Node 0 Active(anon):   10104396 kB
Node 0 Inactive(anon):   511432 kB
Node 0 Active(file):    9527436 kB
Node 0 Inactive(file):  7577664 kB
Node 0 Unevictable:      637864 kB
Node 0 Mlocked:               0 kB
Node 0 Dirty:              1140 kB
Node 0 Writeback:             0 kB
Node 0 FilePages:      18206092 kB
Node 0 Mapped:          2000244 kB
Node 0 AnonPages:      10152780 kB
Node 0 Shmem:           1249348 kB
Node 0 KernelStack:       37440 kB
Node 0 PageTables:       110460 kB
Node 0 NFS_Unstable:          0 kB
Node 0 Bounce:                0 kB
Node 0 WritebackTmp:          0 kB
Node 0 KReclaimable:     843624 kB
Node 0 Slab:            1198060 kB
Node 0 SReclaimable:     843624 kB
Node 0 SUnreclaim:       354436 kB
Node 0 AnonHugePages:     26624 kB
Node 0 ShmemHugePages:        0 kB
Node 0 ShmemPmdMapped:        0 kB
Node 0 FileHugePages:        0 kB
Node 0 FilePmdMapped:        0 kB
Node 0 HugePages_Total:     0
Node 0 HugePages_Free:      0
Node 0 HugePages_Surp:      0`

func TestMemoryForNode(t *testing.T) {
	rootDir, err := os.MkdirTemp("", "fakememory")
	if err != nil {
		t.Errorf("failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(rootDir) // clean up

	if err := makeMemoryTree(rootDir, 2); err != nil {
		t.Errorf("failed to setup the fake tree on %q: %v", rootDir, err)
	}

	memResource := string(v1.ResourceMemory)
	memoryCounters, err := GetMemoryResourceCounters(Handle{rootDir})
	if memoryCounters["memory"][0] != 32718644*1024 {
		t.Errorf("found unexpected amount of memory under the NUMA node 0: %d", memoryCounters[memResource][0])
	}

	if memoryCounters["memory"][1] != 32718644*1024 {
		t.Errorf("found unexpected amount of memory under the NUMA node 1: %d", memoryCounters[memResource][1])
	}
}

func makeMemoryTree(root string, numNodes int) error {
	hnd := Handle{root}
	for idx := 0; idx < numNodes; idx++ {
		if err := os.MkdirAll(hnd.SysDevicesNodesNodeNth(idx), 0755); err != nil {
			return err
		}
		path := filepath.Join(
			hnd.SysDevicesNodesNodeNth(idx),
			"meminfo",
		)
		if err := os.WriteFile(path, []byte(testMeminfo), 0644); err != nil {
			return err
		}

		hpPathSizes := []string{"hugepages-1048576kB", "hugepages-2048kB"}
		for _, hpPathSize := range hpPathSizes {
			hpPath := filepath.Join(
				hnd.SysDevicesNodesNodeNth(idx),
				"hugepages",
				hpPathSize,
			)
			if err := os.MkdirAll(hpPath, 0755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(hpPath, "nr_hugepages"), []byte("16"), 0644); err != nil {
				return err
			}
		}
	}
	return nil
}
