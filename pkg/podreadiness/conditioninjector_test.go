package podreadiness

import (
	"context"
	"fmt"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestEqualPodCondition(t *testing.T) {
	baseCond := makeBaseCondition()

	testCases := []struct {
		name     string
		a, b     v1.PodCondition
		expected bool
	}{
		{
			name:     "identical conditions",
			a:        *baseCond.DeepCopy(),
			b:        *baseCond.DeepCopy(),
			expected: true,
		},
		{
			name:     "different LastTransitionTime",
			a:        *baseCond.DeepCopy(),
			b:        makeBaseCondition(),
			expected: true,
		},
		{
			name: "different LastProbeTime",
			a:    *baseCond.DeepCopy(),
			b: v1.PodCondition{
				Type:          baseCond.Type,
				Status:        baseCond.Status,
				Reason:        baseCond.Reason,
				Message:       baseCond.Message,
				LastProbeTime: metav1.Time{Time: time.Now().Add(10 * time.Second)},
			},
			expected: true,
		},
		{
			name: "different Status",
			a:    *baseCond.DeepCopy(),
			b: v1.PodCondition{
				Type:    baseCond.Type,
				Status:  v1.ConditionFalse,
				Reason:  baseCond.Reason,
				Message: baseCond.Message,
			},
			expected: false,
		},
		{
			name: "different Type",
			a:    *baseCond.DeepCopy(),
			b: v1.PodCondition{
				Type:    v1.PodConditionType(NodeTopologyUpdated),
				Status:  baseCond.Status,
				Reason:  baseCond.Reason,
				Message: baseCond.Message,
			},
			expected: false,
		},
		{
			name: "different Reason",
			a:    *baseCond.DeepCopy(),
			b: v1.PodCondition{
				Type:    baseCond.Type,
				Status:  baseCond.Status,
				Reason:  "ScanFailed",
				Message: baseCond.Message,
			},
			expected: false,
		},
		{
			name: "different Message",
			a:    *baseCond.DeepCopy(),
			b: v1.PodCondition{
				Type:    baseCond.Type,
				Status:  baseCond.Status,
				Reason:  baseCond.Reason,
				Message: "something else happened",
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := equalPodCondition(tc.a, tc.b)
			if got != tc.expected {
				t.Errorf("equalPodCondition() = %v, expected %v", got, tc.expected)
			}
		})
	}
}

const (
	testNS      = "test-ns"
	testPodName = "test-pod"
)

func TestInject_CacheHit(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPodName,
			Namespace: testNS,
		},
	}

	cs := fake.NewSimpleClientset(pod)
	ci := NewConditionInjectorWithIdentity(cs, pod.Namespace, pod.Name)

	cond := v1.PodCondition{
		Type:               v1.PodConditionType(PodresourcesFetched),
		Status:             v1.ConditionTrue,
		LastTransitionTime: metav1.Time{Time: time.Now()},
	}

	ctx := context.Background()

	// First call: cache is empty, should hit the API (get + update-status)
	if err := ci.Inject(ctx, cond); err != nil {
		t.Fatalf("first Inject failed: %v", err)
	}

	checkAPIServerUpdated(t, cs.Actions())

	cs.ClearActions()

	// Second call: same condition (different timestamp), cache should short-circuit
	cond2 := v1.PodCondition{
		Type:               v1.PodConditionType(PodresourcesFetched),
		Status:             v1.ConditionTrue,
		LastTransitionTime: metav1.Time{Time: time.Now().Add(10 * time.Second)},
	}

	if err := ci.Inject(ctx, cond2); err != nil {
		t.Fatalf("second Inject failed: %v", err)
	}

	checkNoAPICalls(t, cs.Actions())
}

func TestInject_CacheMissOnStateChange(t *testing.T) {
	//` — call `Inject` with `Status: True`, then with `Status: False`. Second call must hit the API (Get + UpdateStatus). Confirms real state changes are not suppressed by the cache.
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPodName,
			Namespace: testNS,
		},
	}

	cs := fake.NewSimpleClientset(pod)
	ci := NewConditionInjectorWithIdentity(cs, pod.Namespace, pod.Name)

	cond := v1.PodCondition{
		Type:               v1.PodConditionType(PodresourcesFetched),
		Status:             v1.ConditionTrue,
		LastTransitionTime: metav1.Time{Time: time.Now()},
	}

	ctx := context.Background()

	// First call: cache is empty, should hit the API (get + update-status)
	if err := ci.Inject(ctx, cond); err != nil {
		t.Fatalf("first Inject failed: %v", err)
	}

	checkAPIServerUpdated(t, cs.Actions())

	cs.ClearActions()

	// Second call: condition flips to False, cache should pass through the API server
	cond2 := v1.PodCondition{
		Type:               v1.PodConditionType(PodresourcesFetched),
		Status:             v1.ConditionFalse,
		LastTransitionTime: metav1.Time{Time: time.Now().Add(10 * time.Second)},
	}

	if err := ci.Inject(ctx, cond2); err != nil {
		t.Fatalf("second Inject failed: %v", err)
	}

	checkAPIServerUpdated(t, cs.Actions())

	cs.ClearActions()

	// Third call: at steady state again, cache should short-circuit
	cond3 := v1.PodCondition{
		Type:               v1.PodConditionType(PodresourcesFetched),
		Status:             v1.ConditionFalse,
		LastTransitionTime: metav1.Time{Time: time.Now().Add(20 * time.Second)},
	}

	if err := ci.Inject(ctx, cond3); err != nil {
		t.Fatalf("third Inject failed: %v", err)
	}

	checkNoAPICalls(t, cs.Actions())
}

func TestInject_CacheNotPopulatedOnFailure(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPodName,
			Namespace: testNS,
		},
	}

	cs := fake.NewSimpleClientset(pod)

	called := false
	cs.PrependReactor("update", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "status" {
			return false, nil, nil
		}
		if !called {
			called = true
			return true, nil, fmt.Errorf("injected error")
		}
		return false, nil, nil
	})

	ci := NewConditionInjectorWithIdentity(cs, pod.Namespace, pod.Name)

	cond := v1.PodCondition{
		Type:               v1.PodConditionType(PodresourcesFetched),
		Status:             v1.ConditionTrue,
		LastTransitionTime: metav1.Time{Time: time.Now()},
	}

	ctx := context.Background()

	// First call: cache is empty, should fail

	if err := ci.Inject(ctx, cond); err == nil {
		t.Fatalf("first Inject succeeded, it should fail")
	}

	checkAPIServerUpdated(t, cs.Actions())

	cs.ClearActions()

	// Second call: same condition with different timestamp, but cache expected empty, should hit the API (get + update-status)
	cond2 := v1.PodCondition{
		Type:               v1.PodConditionType(PodresourcesFetched),
		Status:             v1.ConditionTrue,
		LastTransitionTime: metav1.Time{Time: time.Now().Add(10 * time.Second)},
	}

	if err := ci.Inject(ctx, cond2); err != nil {
		t.Fatalf("second Inject failed: %v", err)
	}

	checkAPIServerUpdated(t, cs.Actions())

	cs.ClearActions()

	// Third call: at steady state again, cache should short-circuit
	cond3 := v1.PodCondition{
		Type:               v1.PodConditionType(PodresourcesFetched),
		Status:             v1.ConditionTrue,
		LastTransitionTime: metav1.Time{Time: time.Now().Add(20 * time.Second)},
	}

	if err := ci.Inject(ctx, cond3); err != nil {
		t.Fatalf("third Inject failed: %v", err)
	}

	checkNoAPICalls(t, cs.Actions())
}

func TestInject_ServerAlreadyHasStatus(t *testing.T) {
	cond := v1.PodCondition{
		Type:               v1.PodConditionType(PodresourcesFetched),
		Status:             v1.ConditionTrue,
		LastTransitionTime: metav1.Time{Time: time.Now()},
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPodName,
			Namespace: testNS,
		},
		Status: v1.PodStatus{
			Conditions: []v1.PodCondition{cond},
		},
	}

	cs := fake.NewSimpleClientset(pod)
	ci := NewConditionInjectorWithIdentity(cs, pod.Namespace, pod.Name)

	ctx := context.Background()

	// First call: server already has the condition with matching status
	if err := ci.Inject(ctx, cond); err != nil {
		t.Fatalf("first Inject failed: %v", err)
	}

	checkAPIServerUpdated(t, cs.Actions())

	cs.ClearActions()

	// Second call: cache populated by get matching the desired state, so no more API calls are done
	cond2 := v1.PodCondition{
		Type:               v1.PodConditionType(PodresourcesFetched),
		Status:             v1.ConditionTrue,
		LastTransitionTime: metav1.Time{Time: time.Now().Add(10 * time.Second)},
	}

	if err := ci.Inject(ctx, cond2); err != nil {
		t.Fatalf("second Inject failed: %v", err)
	}

	checkNoAPICalls(t, cs.Actions())
}

func checkAPIServerUpdated(t *testing.T, actions []k8stesting.Action) {
	t.Helper()

	if len(actions) != 2 {
		t.Fatalf("expected 2 actions after first Inject, got %d: %v", len(actions), actions)
	}
	if verb := actions[0].GetVerb(); verb != "get" {
		t.Errorf("first action: expected verb \"get\", got %q", verb)
	}
	if verb := actions[1].GetVerb(); verb != "update" {
		t.Errorf("second action: expected verb \"update\", got %q", verb)
	}
	if sub := actions[1].GetSubresource(); sub != "status" {
		t.Errorf("second action: expected subresource \"status\", got %q", sub)
	}
}

func checkNoAPICalls(t *testing.T, actions []k8stesting.Action) {
	if len(actions) != 0 {
		t.Fatalf("expected still no actions after second Inject (cache hit), got %v", actions)
	}
}

func makeBaseCondition() v1.PodCondition {
	return v1.PodCondition{
		Type:               v1.PodConditionType(PodresourcesFetched),
		Status:             v1.ConditionTrue,
		Reason:             "ScanOK",
		Message:            "scan succeeded",
		LastTransitionTime: metav1.Time{Time: time.Now()},
	}
}
