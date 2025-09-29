/*
Copyright 2025 The Kubernetes Authors.

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

package pfpdump

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/k8stopologyawareschedwg/podfingerprint"
)

func TestToFile(t *testing.T) {
	tmpDir := t.TempDir()
	pfp := podfingerprint.Status{
		FingerprintExpected: "pfpv0expected",
		FingerprintComputed: "pfpv0computed",
		Pods: []podfingerprint.NamespacedName{
			{
				Namespace: "foo-1",
				Name:      "bar-1",
			},
			{
				Namespace: "foo-2",
				Name:      "bar-1",
			},
			{
				Namespace: "foo-2",
				Name:      "bar-2",
			},
		},
		NodeName: "test-node.ci.dev",
	}
	err := ToFile(pfp, tmpDir, "pfpdump.json")
	if err != nil {
		t.Fatalf("ToFile(%s, %s) failed: %v", tmpDir, "pfpdump.json", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "pfpdump.json"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var pfp2 podfingerprint.Status
	err = json.Unmarshal(data, &pfp2)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if diff := cmp.Diff(pfp, pfp2); diff != "" {
		t.Fatalf("roundtrip error: diff=%v", diff)
	}
}
