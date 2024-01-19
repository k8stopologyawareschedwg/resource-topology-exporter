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

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetDefaults(t *testing.T) {
	closer := setupTest(t)
	t.Cleanup(closer)

	pArgs := ProgArgs{}
	SetDefaults(&pArgs)

	pArgsAsJson := pArgs.ToJSONString()
	var err error

	golden := filepath.Join(testDataDir, fmt.Sprintf("%s.expected.json", t.Name()))
	if *update {
		err = os.WriteFile(golden, []byte(pArgsAsJson), 0644)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	expectedAsJson, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// stray newlines make the failure unnecessarily hard to debug
	got := strings.TrimSpace(string(pArgsAsJson))
	expected := strings.TrimSpace(string(expectedAsJson))
	if got != expected {
		t.Errorf("invalid defaults.\n>>> got:\n{%s}\n>>> expected:\n{%s}", got, expected)
	}
}
