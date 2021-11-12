package utils

import (
	"testing"

	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
)

func TestDump(t *testing.T) {
	type testCase struct {
		name     string
		obj      interface{}
		expected string
	}

	testCases := []testCase{
		{
			name:     "nil",
			obj:      nil,
			expected: "null\n",
		},
		{
			name:     "empty topology zone",
			obj:      v1alpha1.Zone{},
			expected: "name: \"\"\ntype: \"\"\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := Dump(tc.obj)
			if tc.expected != got {
				t.Fatalf("Dump(%s) error expected=%q got=%q", tc.name, tc.expected, got)
			}
		})
	}
}
