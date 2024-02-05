/*
 * Copyright 2023 The Kubernetes Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package podres

import (
	"errors"
	"testing"
)

func TestParseEndpoint(t *testing.T) {
	type testCase struct {
		name          string
		endpoint      string
		expectedPath  string
		expectedError error
	}

	testCases := []testCase{
		{
			name:          "empty",
			expectedError: UnsupportedProtocolError{},
		},
		{
			name:          "bad proto",
			endpoint:      "foobar:///path",
			expectedError: UnsupportedProtocolError{proto: "foobar"},
		},
		{
			name:     "good proto",
			endpoint: "unix://",
		},
		{
			name:         "good proto, path given",
			endpoint:     "unix:///my/path",
			expectedPath: "/my/path",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, got, err := ParseEndpoint(tc.endpoint)
			if !errors.Is(err, tc.expectedError) {
				t.Fatalf("ParseEndpoint failed err=%v expected=%v", err, tc.expectedError)
			}
			if got != tc.expectedPath {
				t.Fatalf("ParseEndpoint failed path=%q expected=%q", got, tc.expectedPath)
			}
		})
	}
}
