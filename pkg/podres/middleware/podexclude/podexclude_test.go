/*
 * Copyright 2022 Red Hat, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package podexclude

import (
	"testing"
)

func TestShouldExclude(t *testing.T) {
	var testCases = []struct {
		name         string
		podExcludes  List
		podNamespace string
		podName      string
		excluded     bool
	}{
		{
			name:         "no excludes",
			podExcludes:  List{},
			podNamespace: "foo-bar",
			podName:      "quux-bar",
			excluded:     false,
		},
		{
			name: "name no match",
			podExcludes: List{
				{
					NamespacePattern: "foo-bar",
					NamePattern:      "baz",
				},
			},
			podNamespace: "foo-bar",
			podName:      "quux-bar",
			excluded:     false,
		},
		{
			name: "name no match, star",
			podExcludes: List{
				{
					NamespacePattern: "foo-bar",
					NamePattern:      "baz-*",
				},
			},
			podNamespace: "foo-bar",
			podName:      "quux-bar",
			excluded:     false,
		},
		{
			name: "name match",
			podExcludes: List{
				{
					NamespacePattern: "foo-bar",
					NamePattern:      "quux-bar",
				},
			},
			podNamespace: "foo-bar",
			podName:      "quux-bar",
			excluded:     true,
		},
		{
			name: "name match, star",
			podExcludes: List{
				{
					NamespacePattern: "foo-bar",
					NamePattern:      "*-bar",
				},
			},
			podNamespace: "foo-bar",
			podName:      "quux-bar",
			excluded:     true,
		},

		{
			name: "namespace no match",
			podExcludes: List{
				{
					NamespacePattern: "blah",
					NamePattern:      "quux-bar",
				},
			},
			podNamespace: "foo-bar",
			podName:      "quux-bar",
			excluded:     false,
		},
		{
			name: "namespace no match, star",
			podExcludes: List{
				{
					NamespacePattern: "blah-*",
					NamePattern:      "quux-bar",
				},
			},
			podNamespace: "foo-bar",
			podName:      "quux-bar",
			excluded:     false,
		},
		{
			name: "namespace match",
			podExcludes: List{
				{
					NamespacePattern: "foo-bar",
					NamePattern:      "quux-bar",
				},
			},
			podNamespace: "foo-bar",
			podName:      "quux-bar",
			excluded:     true,
		},
		{
			name: "namespace match, star",
			podExcludes: List{
				{
					NamespacePattern: "foo-*",
					NamePattern:      "quux-bar",
				},
			},
			podNamespace: "foo-bar",
			podName:      "quux-bar",
			excluded:     true,
		},
		{
			name: "everything",
			podExcludes: List{
				{
					NamespacePattern: "*",
					NamePattern:      "*",
				},
			},
			podNamespace: "foo-bar",
			podName:      "quux-bar",
			excluded:     true,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			debug := true
			exc := ShouldExclude(testCase.podExcludes, testCase.podNamespace, testCase.podName, debug)
			if exc != testCase.excluded {
				t.Errorf("excluded=%v expected=%v for %s/%s with %v", exc, testCase.excluded, testCase.podNamespace, testCase.podName, testCase.podExcludes)
			}
		})
	}
}
