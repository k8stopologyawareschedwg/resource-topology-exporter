package podreadiness

import (
	"testing"

	v1 "k8s.io/api/core/v1"
)

func TestSetCondition(t *testing.T) {
	type testCase struct {
		name     string
		status   v1.ConditionStatus
		cType    RTEConditionType
		expected v1.PodCondition
	}

	c := make(chan v1.PodCondition, 0)

	testCases := []testCase{
		{
			name:     "NodeTopologyUpdated condition is true",
			status:   v1.ConditionTrue,
			cType:    NodeTopologyUpdated,
			expected: newConditionTemplate(NodeTopologyUpdated, v1.ConditionTrue),
		},
		{
			name:     "NodeTopologyUpdated condition is false",
			status:   v1.ConditionFalse,
			cType:    NodeTopologyUpdated,
			expected: newConditionTemplate(NodeTopologyUpdated, v1.ConditionTrue),
		},
		{
			name:     "PodresourcesFetched condition is true",
			status:   v1.ConditionTrue,
			cType:    PodresourcesFetched,
			expected: newConditionTemplate(NodeTopologyUpdated, v1.ConditionTrue),
		},
		{
			name:     "PodresourcesFetched condition is false",
			status:   v1.ConditionFalse,
			cType:    PodresourcesFetched,
			expected: newConditionTemplate(NodeTopologyUpdated, v1.ConditionTrue),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			go SetCondition(c, tc.cType, tc.status)
			cond := <-c
			if cond.Status != tc.status {
				t.Errorf("expected status %q, got %q", tc.status, cond.Status)
			}
			if RTEConditionType(cond.Type) != tc.cType {
				t.Errorf("expected type %q, got %q", tc.cType, cond.Type)
			}
		})
	}
}
