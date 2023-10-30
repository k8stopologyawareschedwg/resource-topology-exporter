/*
Copyright 2023 The Kubernetes Authors.

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

package metrics

import (
	"os"
	"testing"
)

func TestSetup(t *testing.T) {
	type testCase struct {
		name     string
		arg      string
		env      string
		expected string
	}

	for _, tcase := range []testCase{
		{
			name:     "all empty",
			expected: mustHostname(),
		},
		{
			name:     "from arg",
			arg:      "arg.localtest.it",
			expected: "arg.localtest.it",
		},
		{
			name:     "from env",
			env:      "env.localtest.it",
			expected: "env.localtest.it",
		},
		{
			name:     "overriding order",
			arg:      "arg.localtest.it",
			env:      "env.localtest.it",
			expected: "arg.localtest.it",
		},
	} {
		t.Run(tcase.name, func(t *testing.T) {
			if tcase.env != "" {
				t.Setenv("NODE_NAME", tcase.env)
			}

			err := Setup(tcase.arg)
			if err != nil {
				t.Fatalf("Setup failed: %v", err)
			}
			got := GetNodeName()
			if got != tcase.expected {
				t.Errorf("invalid hostname: got %q expected %q", got, tcase.expected)
			}
		})
	}
}

func mustHostname() string {
	val, err := os.Hostname()
	if err != nil {
		panic(err)
	}
	return val
}
