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
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientk8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha2"
	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/clientset/versioned/fake"
)

func TestUpdateTMPolicy(t *testing.T) {
	nodeName := "test-node"

	args := Args{
		Hostname: nodeName,
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
	nrtUpd = NewNRTUpdater(nodeGetter, args, tmConfInitial)
	err = nrtUpd.UpdateWithClient(
		cli,
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

	nrtResource := schema.GroupVersionResource{Group: "topology.node.k8s.io", Version: "v1alpha2", Resource: "noderesourcetopologies"}
	obj, err := cli.Tracker().Get(nrtResource, "", nodeName)
	if err != nil {
		t.Fatalf("failed to get the NRT object from tracker: %v", err)
	}
	checkTMConfig(t, obj, tmConfInitial)

	nrtUpd = NewNRTUpdater(nodeGetter, args, tmConfUpdated)
	err = nrtUpd.UpdateWithClient(
		cli,
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
	nodeName := "test-node"

	args := Args{
		Hostname: nodeName,
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
	nrtUpd = NewNRTUpdater(nodeGetter, args, tmConfig)

	err = nrtUpd.UpdateWithClient(
		cli,
		MonitorInfo{Zones: v1alpha2.ZoneList{zoneInfo}},
	)
	if err != nil {
		t.Fatalf("failed to perform the initial creation: %v", err)
	}

	nrtResource := schema.GroupVersionResource{Group: "topology.node.k8s.io", Version: "v1alpha2", Resource: "noderesourcetopologies"}
	obj, err := cli.Tracker().Get(nrtResource, "", nodeName)
	if err != nil {
		t.Fatalf("failed to get the NRT object from tracker: %v", err)
	}
	checkOwnerReferences(t, obj, expected)

	err = nrtUpd.UpdateWithClient(
		cli,
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
