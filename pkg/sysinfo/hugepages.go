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
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"k8s.io/klog/v2"

	"k8s.io/apimachinery/pkg/api/resource"
)

// TODO review
type Hugepages struct {
	NodeID int
	SizeKB int
	Total  int
}

type PerNUMACounters map[int]int64

func GetHugepages(hnd Handle) ([]Hugepages, error) {
	entries, err := os.ReadDir(hnd.SysDevicesNodes())
	if err != nil {
		return nil, err
	}

	hugepages := []Hugepages{}
	for _, entry := range entries {
		entryName := entry.Name()
		if entry.IsDir() && strings.HasPrefix(entryName, "node") {
			nodeID, err := strconv.Atoi(entryName[4:])
			if err != nil {
				return hugepages, fmt.Errorf("cannot detect the node ID for %q", entryName)
			}
			nodeHugepages, err := HugepagesForNode(hnd, nodeID)
			if err != nil {
				return hugepages, fmt.Errorf("cannot find the hugepages on NUMA node %d: %w", nodeID, err)
			}
			hugepages = append(hugepages, nodeHugepages...)
		}
	}
	return hugepages, nil
}

func HugepagesForNode(hnd Handle, nodeID int) ([]Hugepages, error) {
	hpPath := filepath.Join(
		hnd.SysDevicesNodesNodeNth(nodeID),
		"hugepages",
	)
	hugepages := []Hugepages{}

	entries, err := os.ReadDir(hpPath)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		entryName := entry.Name()
		if !entry.IsDir() {
			klog.Warningf("unexpected entry in %q: %q - skipped", hpPath, entryName)
			continue
		}

		var hugepageSizeKB int
		if n, err := fmt.Sscanf(entryName, "hugepages-%dkB", &hugepageSizeKB); n != 1 || err != nil {
			return hugepages, fmt.Errorf("malformed hugepages entry %q", entryName)
		}

		entryPath := filepath.Join(hpPath, entryName)
		hpCountPath, err := filepath.EvalSymlinks(filepath.Join(entryPath, "nr_hugepages"))
		if err != nil {
			return hugepages, fmt.Errorf("cannot clean %q: %w", entryPath, err)
		}

		// TODO: use filepath.Rel?
		if !strings.HasPrefix(hpCountPath, hpPath) {
			return hugepages, fmt.Errorf("unexpected path resolution: %q not subpath of %q", hpCountPath, hpPath)
		}

		totalCount, err := readIntFromFile(hpCountPath)
		if err != nil {
			return hugepages, fmt.Errorf("cannot read from %q: %w", hpCountPath, err)
		}

		hugepages = append(hugepages, Hugepages{
			NodeID: nodeID,
			SizeKB: hugepageSizeKB,
			Total:  totalCount,
		})
	}

	return hugepages, nil
}

func readIntFromFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return -1, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

func HugepageResourceNameFromSize(sizeKB int) string {
	qty := resource.NewQuantity(int64(sizeKB*1024), resource.BinarySI)
	return "hugepages-" + qty.String()
}
