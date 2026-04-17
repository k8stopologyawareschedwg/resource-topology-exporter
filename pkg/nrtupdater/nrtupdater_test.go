/*
Copyright 2022 The Kubernetes Authors.

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

package nrtupdater

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientk8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	jsonpatch "github.com/evanphx/json-patch/v5"

	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha2"
	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/clientset/versioned/fake"
)

var nrtResource = schema.GroupVersionResource{Group: "topology.node.k8s.io", Version: "v1alpha2", Resource: "noderesourcetopologies"}

type updateModeTestCase struct {
	name     string
	patchMode bool
}

var updateModes = []updateModeTestCase{
	{name: "update", patchMode: false},
	{name: "patch", patchMode: true},
}

func TestUpdateTMPolicy(t *testing.T) {
	for _, mode := range updateModes {
		t.Run(mode.name, func(t *testing.T) {
			testUpdateTMPolicy(t, mode.patchMode)
		})
	}
}

func testUpdateTMPolicy(t *testing.T, patchMode bool) {
	t.Helper()

	nodeName := "test-node"

	args := Args{
		Hostname: nodeName,
		PatchMode: patchMode,
	}
	var nrtUpd *NRTUpdater
	cli := fake.NewSimpleClientset()

	tmConfInitial := TMConfig{
		Scope:  "scope-initial",
		Policy: "policy-initial",
	}
	tmConfUpdated := TMConfig{
		Scope:  "scope-updated",
		Policy: "polcy-updated",
	}

	var err error
	k8sClient := clientk8sfake.NewSimpleClientset()
	nodeGetter, err := NewCachedNodeGetter(k8sClient, context.Background())
	if err != nil {
		t.Fatalf("failed to create node getter: %v", err)
	}
	nrtUpd, err = NewNRTUpdater(nodeGetter, cli, args, tmConfInitial)
	if err != nil {
		t.Fatalf("failed to create NRT updater: %v", err)
	}
	err = nrtUpd.Update(
		context.TODO(),
		MonitorInfo{
			Zones: v1alpha2.ZoneList{
				{
					Name: "test-zone-0",
					Type: "node",
					Resources: v1alpha2.ResourceInfoList{
						{
							Name:        string(corev1.ResourceCPU),
							Capacity:    resource.MustParse("16"),
							Allocatable: resource.MustParse("14"),
							Available:   resource.MustParse("14"),
						},
						{
							Name:        string(corev1.ResourceMemory),
							Capacity:    resource.MustParse("32Gi"),
							Allocatable: resource.MustParse("30Gi"),
							Available:   resource.MustParse("30Gi"),
						},
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("failed to perform the initial creation: %v", err)
	}

	obj, err := cli.Tracker().Get(nrtResource, "", nodeName)
	if err != nil {
		t.Fatalf("failed to get the NRT object from tracker: %v", err)
	}
	checkTMConfig(t, obj, tmConfInitial)

	nrtUpd, err = NewNRTUpdater(nodeGetter, cli, args, tmConfUpdated)
	if err != nil {
		t.Fatalf("failed to create NRT updater: %v", err)
	}
	err = nrtUpd.Update(
		context.TODO(),
		MonitorInfo{
			Zones: v1alpha2.ZoneList{
				{
					Name: "test-zone-0",
					Type: "node",
					Resources: v1alpha2.ResourceInfoList{
						{
							Name:        string(corev1.ResourceCPU),
							Capacity:    resource.MustParse("16"),
							Allocatable: resource.MustParse("14"),
							Available:   resource.MustParse("10"),
						},
						{
							Name:        string(corev1.ResourceMemory),
							Capacity:    resource.MustParse("32Gi"),
							Allocatable: resource.MustParse("30Gi"),
							Available:   resource.MustParse("22Gi"),
						},
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("failed to perform the update: %v", err)
	}

	obj, err = cli.Tracker().Get(nrtResource, "", nodeName)
	if err != nil {
		t.Fatalf("failed to get the NRT object from tracker: %v", err)
	}
	checkTMConfig(t, obj, tmConfUpdated)
}

func TestUpdateOwnerReferences(t *testing.T) {
	for _, mode := range updateModes {
		t.Run(mode.name, func(t *testing.T) {
			testUpdateOwnerReferences(t, mode.patchMode)
		})
	}
}

func testUpdateOwnerReferences(t *testing.T, patchMode bool) {
	t.Helper()

	nodeName := "test-node"

	args := Args{
		Hostname: nodeName,
		PatchMode: patchMode,
	}
	tmConfig := TMConfig{
		Scope:  "scope-whatever",
		Policy: "policy-whatever",
	}

	zoneInfo := v1alpha2.Zone{
		Name: "test-zone-0",
		Type: "node",
		Resources: v1alpha2.ResourceInfoList{
			{
				Name:        string(corev1.ResourceCPU),
				Capacity:    resource.MustParse("16"),
				Allocatable: resource.MustParse("14"),
				Available:   resource.MustParse("14"),
			},
			{
				Name:        string(corev1.ResourceMemory),
				Capacity:    resource.MustParse("32Gi"),
				Allocatable: resource.MustParse("30Gi"),
				Available:   resource.MustParse("30Gi"),
			},
		},
	}

	node := corev1.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Node",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
		},
	}

	expected := metav1.OwnerReference{
		Kind:       node.Kind,
		Name:       node.Name,
		APIVersion: node.APIVersion,
		UID:        node.UID,
	}

	var nrtUpd *NRTUpdater

	cli := fake.NewSimpleClientset()
	var err error
	k8sClient := clientk8sfake.NewSimpleClientset(&node)
	nodeGetter, err := NewCachedNodeGetter(k8sClient, context.Background())
	if err != nil {
		t.Fatalf("failed to create node getter: %v", err)
	}
	nrtUpd, err = NewNRTUpdater(nodeGetter, cli, args, tmConfig)
	if err != nil {
		t.Fatalf("failed to create node getter: %v", err)
	}

	err = nrtUpd.Update(
		context.TODO(),
		MonitorInfo{Zones: v1alpha2.ZoneList{zoneInfo}},
	)
	if err != nil {
		t.Fatalf("failed to perform the initial creation: %v", err)
	}

	obj, err := cli.Tracker().Get(nrtResource, "", nodeName)
	if err != nil {
		t.Fatalf("failed to get the NRT object from tracker: %v", err)
	}
	checkOwnerReferences(t, obj, expected)

	err = nrtUpd.Update(
		context.TODO(),
		MonitorInfo{Zones: v1alpha2.ZoneList{zoneInfo}},
	)
	if err != nil {
		t.Fatalf("failed to perform the initial creation: %v", err)
	}
	obj, err = cli.Tracker().Get(nrtResource, "", nodeName)
	if err != nil {
		t.Fatalf("failed to get the NRT object from tracker: %v", err)
	}
	checkOwnerReferences(t, obj, expected)
}

func makeBaseNRT() *v1alpha2.NodeResourceTopology {
	return &v1alpha2.NodeResourceTopology{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				"rte.update": "periodic",
			},
		},
		Zones: v1alpha2.ZoneList{
			{
				Name: "zone-0",
				Type: "node",
				Resources: v1alpha2.ResourceInfoList{
					{
						Name:        string(corev1.ResourceCPU),
						Capacity:    resource.MustParse("16"),
						Allocatable: resource.MustParse("14"),
						Available:   resource.MustParse("14"),
					},
					{
						Name:        string(corev1.ResourceMemory),
						Capacity:    resource.MustParse("32Gi"),
						Allocatable: resource.MustParse("30Gi"),
						Available:   resource.MustParse("30Gi"),
					},
				},
			},
		},
		Attributes: v1alpha2.AttributeList{
			{Name: "topologyManagerScope", Value: "container"},
			{Name: "topologyManagerPolicy", Value: "single-numa-node"},
		},
	}
}

func applyMergePatch(t *testing.T, original *v1alpha2.NodeResourceTopology, patch []byte) *v1alpha2.NodeResourceTopology {
	t.Helper()
	origJSON, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal original: %v", err)
	}
	patchedJSON, err := jsonpatch.MergePatch(origJSON, patch)
	if err != nil {
		t.Fatalf("failed to apply merge patch: %v", err)
	}
	var result v1alpha2.NodeResourceTopology
	if err := json.Unmarshal(patchedJSON, &result); err != nil {
		t.Fatalf("failed to unmarshal patched result: %v", err)
	}
	return &result
}

func TestMakeNRTPatch(t *testing.T) {
	t.Run("identical objects produce empty patch", func(t *testing.T) {
		nrtOld := makeBaseNRT()
		nrtNew := nrtOld.DeepCopy()

		patchInfo, reason, err := MakeNRTPatch(nrtOld, nrtNew)
		if err != nil {
			t.Fatalf("unexpected error (reason=%q): %v", reason, err)
		}
		if reason != "" {
			t.Errorf("expected empty reason, got %q", reason)
		}
		if string(patchInfo.Patch) != "{}" {
			t.Errorf("expected empty patch for identical objects, got: %s", string(patchInfo.Patch))
		}
	})

	t.Run("changed resource available", func(t *testing.T) {
		nrtOld := makeBaseNRT()
		nrtNew := nrtOld.DeepCopy()
		nrtNew.Zones[0].Resources[0].Available = resource.MustParse("10")

		patchInfo, reason, err := MakeNRTPatch(nrtOld, nrtNew)
		if err != nil {
			t.Fatalf("unexpected error (reason=%q): %v", reason, err)
		}
		if string(patchInfo.Patch) == "{}" {
			t.Fatalf("expected non-empty patch for changed resource")
		}

		result := applyMergePatch(t, nrtOld, patchInfo.Patch)
		if !result.Zones[0].Resources[0].Available.Equal(resource.MustParse("10")) {
			t.Errorf("patch did not update Available: got %v", result.Zones[0].Resources[0].Available.String())
		}
		if !result.Zones[0].Resources[1].Available.Equal(resource.MustParse("30Gi")) {
			t.Errorf("patch unexpectedly changed unmodified resource: got %v", result.Zones[0].Resources[1].Available.String())
		}
	})

	t.Run("added zone", func(t *testing.T) {
		nrtOld := makeBaseNRT()
		nrtNew := nrtOld.DeepCopy()
		nrtNew.Zones = append(nrtNew.Zones, v1alpha2.Zone{
			Name: "zone-1",
			Type: "node",
			Resources: v1alpha2.ResourceInfoList{
				{
					Name:        string(corev1.ResourceCPU),
					Capacity:    resource.MustParse("8"),
					Allocatable: resource.MustParse("8"),
					Available:   resource.MustParse("6"),
				},
			},
		})

		patchInfo, reason, err := MakeNRTPatch(nrtOld, nrtNew)
		if err != nil {
			t.Fatalf("unexpected error (reason=%q): %v", reason, err)
		}
		if string(patchInfo.Patch) == "{}" {
			t.Fatalf("expected non-empty patch for added zone")
		}

		result := applyMergePatch(t, nrtOld, patchInfo.Patch)
		if len(result.Zones) != 2 {
			t.Fatalf("expected 2 zones after patch, got %d", len(result.Zones))
		}
		if result.Zones[1].Name != "zone-1" {
			t.Errorf("expected added zone name zone-1, got %q", result.Zones[1].Name)
		}
	})

	t.Run("changed annotations", func(t *testing.T) {
		nrtOld := makeBaseNRT()
		nrtNew := nrtOld.DeepCopy()
		nrtNew.Annotations["rte.update"] = "reactive"

		patchInfo, reason, err := MakeNRTPatch(nrtOld, nrtNew)
		if err != nil {
			t.Fatalf("unexpected error (reason=%q): %v", reason, err)
		}
		if string(patchInfo.Patch) == "{}" {
			t.Fatalf("expected non-empty patch for changed annotation")
		}

		result := applyMergePatch(t, nrtOld, patchInfo.Patch)
		if result.Annotations["rte.update"] != "reactive" {
			t.Errorf("patch did not update annotation: got %q", result.Annotations["rte.update"])
		}
	})

	t.Run("changed attributes", func(t *testing.T) {
		nrtOld := makeBaseNRT()
		nrtNew := nrtOld.DeepCopy()
		nrtNew.Attributes[1].Value = "best-effort"

		patchInfo, reason, err := MakeNRTPatch(nrtOld, nrtNew)
		if err != nil {
			t.Fatalf("unexpected error (reason=%q): %v", reason, err)
		}
		if string(patchInfo.Patch) == "{}" {
			t.Fatalf("expected non-empty patch for changed attributes")
		}

		result := applyMergePatch(t, nrtOld, patchInfo.Patch)
		if result.Attributes[1].Value != "best-effort" {
			t.Errorf("patch did not update attribute: got %q", result.Attributes[1].Value)
		}
	})

	t.Run("size ratio is smaller for partial changes", func(t *testing.T) {
		nrtOld := makeBaseNRT()

		nrtSmallChange := nrtOld.DeepCopy()
		nrtSmallChange.Zones[0].Resources[0].Available = resource.MustParse("12")
		smallInfo, _, err := MakeNRTPatch(nrtOld, nrtSmallChange)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		nrtBigChange := nrtOld.DeepCopy()
		nrtBigChange.Zones[0].Resources[0].Available = resource.MustParse("12")
		nrtBigChange.Zones[0].Resources[1].Available = resource.MustParse("20Gi")
		nrtBigChange.Annotations["rte.update"] = "reactive"
		nrtBigChange.Attributes[0].Value = "pod"
		bigInfo, _, err := MakeNRTPatch(nrtOld, nrtBigChange)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if smallInfo.SizeRatio() >= bigInfo.SizeRatio() {
			t.Errorf("expected small change ratio (%.3f) < big change ratio (%.3f)",
				smallInfo.SizeRatio(), bigInfo.SizeRatio())
		}
		if smallInfo.SizeRatio() >= 1.0 {
			t.Errorf("expected small change ratio < 1.0, got %.3f", smallInfo.SizeRatio())
		}
	})

	t.Run("FullObjBytes matches marshaled new object", func(t *testing.T) {
		nrtOld := makeBaseNRT()
		nrtNew := nrtOld.DeepCopy()
		nrtNew.Zones[0].Resources[0].Available = resource.MustParse("10")

		patchInfo, _, err := MakeNRTPatch(nrtOld, nrtNew)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		newJSON, err := json.Marshal(nrtNew)
		if err != nil {
			t.Fatalf("failed to marshal new NRT: %v", err)
		}
		if patchInfo.FullObjBytes != len(newJSON) {
			t.Errorf("FullObjBytes=%d, expected %d (marshaled new object)", patchInfo.FullObjBytes, len(newJSON))
		}
	})
}

func nrtVerbs(actions []k8stesting.Action) []string {
	var verbs []string
	for _, a := range actions {
		if a.GetResource() == nrtResource {
			verbs = append(verbs, a.GetVerb())
		}
	}
	return verbs
}

func TestPatchMode(t *testing.T) {
	nodeName := "test-node"

	args := Args{
		Hostname: nodeName,
		PatchMode: true,
	}
	tmConfig := TMConfig{
		Scope:  "scope-test",
		Policy: "policy-test",
	}

	cli := fake.NewSimpleClientset()
	k8sClient := clientk8sfake.NewSimpleClientset()
	nodeGetter, err := NewCachedNodeGetter(k8sClient, context.Background())
	if err != nil {
		t.Fatalf("failed to create node getter: %v", err)
	}
	nrtUpd, err := NewNRTUpdater(nodeGetter, cli, args, tmConfig)
	if err != nil {
		t.Fatalf("failed to create NRT updater: %v", err)
	}

	// First call: no prevNRT, patch path fails, falls back to update: get (not found) -> create
	err = nrtUpd.Update(context.TODO(), MonitorInfo{Zones: v1alpha2.ZoneList{
		{
			Name: "zone-0",
			Type: "node",
			Resources: v1alpha2.ResourceInfoList{
				{
					Name:        string(corev1.ResourceCPU),
					Capacity:    resource.MustParse("16"),
					Allocatable: resource.MustParse("14"),
					Available:   resource.MustParse("14"),
				},
			},
		},
	}})

	if err != nil {
		t.Fatalf("first update failed: %v", err)
	}

	verbs := nrtVerbs(cli.Actions())
	if !reflect.DeepEqual(verbs, []string{"get", "create"}) {
		t.Errorf("first update: expected get+create fallback, got verbs: %v", verbs)
	}

	// Second call: prevNRT is set, should use patch directly
	cli.ClearActions()

	err = nrtUpd.Update(context.TODO(), MonitorInfo{Zones: v1alpha2.ZoneList{
		{
			Name: "zone-0",
			Type: "node",
			Resources: v1alpha2.ResourceInfoList{
				{
					Name:        string(corev1.ResourceCPU),
					Capacity:    resource.MustParse("16"),
					Allocatable: resource.MustParse("14"),
					Available:   resource.MustParse("10"),
				},
			},
		},
	}})

	if err != nil {
		t.Fatalf("second update failed: %v", err)
	}

	verbs = nrtVerbs(cli.Actions())
	if !reflect.DeepEqual(verbs, []string{"patch"}) {
		t.Errorf("second update: expected patch verb, got verbs: %v", verbs)
	}

	// Verify the final object state
	obj, err := cli.Tracker().Get(nrtResource, "", nodeName)
	if err != nil {
		t.Fatalf("failed to get NRT after second update: %v", err)
	}
	nrtObj := obj.(*v1alpha2.NodeResourceTopology)
	if !nrtObj.Zones[0].Resources[0].Available.Equal(resource.MustParse("10")) {
		t.Errorf("expected Available=10, got %v", nrtObj.Zones[0].Resources[0].Available.String())
	}
}

func TestPatchModeFallbackOnError(t *testing.T) {
	nodeName := "test-node"

	args := Args{
		Hostname: nodeName,
		PatchMode: true,
	}
	tmConfig := TMConfig{
		Scope:  "scope-test",
		Policy: "policy-test",
	}

	cli := fake.NewSimpleClientset()
	k8sClient := clientk8sfake.NewSimpleClientset()
	nodeGetter, err := NewCachedNodeGetter(k8sClient, context.Background())
	if err != nil {
		t.Fatalf("failed to create node getter: %v", err)
	}
	nrtUpd, err := NewNRTUpdater(nodeGetter, cli, args, tmConfig)
	if err != nil {
		t.Fatalf("failed to create NRT updater: %v", err)
	}

	// Bootstrap: create the object and populate prevNRT
	err = nrtUpd.Update(context.TODO(), MonitorInfo{Zones: v1alpha2.ZoneList{
		{
			Name: "zone-0",
			Type: "node",
			Resources: v1alpha2.ResourceInfoList{
				{
					Name:        string(corev1.ResourceCPU),
					Capacity:    resource.MustParse("16"),
					Allocatable: resource.MustParse("14"),
					Available:   resource.MustParse("14"),
				},
			},
		},
	}})
	if err != nil {
		t.Fatalf("bootstrap update failed: %v", err)
	}

	// Inject a patch failure
	cli.PrependReactor("patch", "noderesourcetopologies", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("simulated patch failure")
	})

	cli.ClearActions()

	// Patch should fail, then fall back to get+update
	err = nrtUpd.Update(context.TODO(), MonitorInfo{Zones: v1alpha2.ZoneList{
		{
			Name: "zone-0",
			Type: "node",
			Resources: v1alpha2.ResourceInfoList{
				{
					Name:        string(corev1.ResourceCPU),
					Capacity:    resource.MustParse("16"),
					Allocatable: resource.MustParse("14"),
					Available:   resource.MustParse("8"),
				},
			},
		},
	}})
	if err != nil {
		t.Fatalf("update with patch fallback should succeed: %v", err)
	}

	verbs := nrtVerbs(cli.Actions())
	if !reflect.DeepEqual(verbs, []string{"patch", "get", "update"}) {
		t.Errorf("expected patch failed, then fallback to get+update, got verbs: %v", verbs)
	}

	// Verify final object state is correct despite patch failure
	obj, err := cli.Tracker().Get(nrtResource, "", nodeName)
	if err != nil {
		t.Fatalf("failed to get NRT after fallback: %v", err)
	}
	nrtObj := obj.(*v1alpha2.NodeResourceTopology)
	if !nrtObj.Zones[0].Resources[0].Available.Equal(resource.MustParse("8")) {
		t.Errorf("expected Available=8 after fallback, got %v", nrtObj.Zones[0].Resources[0].Available.String())
	}
}

func checkTMConfig(t *testing.T, obj runtime.Object, expectedConf TMConfig) {
	t.Helper()

	nrtObj, ok := obj.(*v1alpha2.NodeResourceTopology)
	if !ok {
		t.Fatalf("provided object is not a NodeResourceTopology")
	}
	if len(nrtObj.TopologyPolicies) > 01 {
		t.Fatalf("unexpected topology policies: %#v", nrtObj.TopologyPolicies)
	}
	gotConf := tmConfigFromAttributes(nrtObj.Attributes)
	if !reflect.DeepEqual(gotConf, expectedConf) {
		t.Fatalf("config got=%+#v expected=%+#v", gotConf, expectedConf)
	}
}

func tmConfigFromAttributes(attrs v1alpha2.AttributeList) TMConfig {
	conf := TMConfig{}
	for _, attr := range attrs {
		if attr.Name == "topologyManagerScope" {
			conf.Scope = attr.Value
			continue
		}
		if attr.Name == "topologyManagerPolicy" {
			conf.Policy = attr.Value
			continue
		}
	}
	return conf
}

func checkOwnerReferences(t *testing.T, obj runtime.Object, expected metav1.OwnerReference) {
	t.Helper()

	nrtObj, ok := obj.(*v1alpha2.NodeResourceTopology)
	if !ok {
		t.Fatalf("provided object is not a NodeResourceTopology")
	}

	nodeReferences := []metav1.OwnerReference{}
	for _, own := range nrtObj.OwnerReferences {
		if own.Kind == "Node" {
			nodeReferences = append(nodeReferences, own)
		}
	}

	if len(nodeReferences) != 1 {
		t.Fatalf("unexpected number of node OwnerReferences: %#v", nodeReferences)
	}
	if !reflect.DeepEqual(nodeReferences[0], expected) {
		t.Fatalf("unexpected node OwnerReference. got=%+#v expected=%+#v", nodeReferences[0], expected)
	}
}
