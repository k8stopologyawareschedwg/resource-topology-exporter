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
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/clientset/versioned/fake"
)

func TestUpdateTMPolicy(t *testing.T) {
	nodeName := "test-node"

	args := Args{
		Hostname: nodeName,
	}
	var nrtUpd *NRTUpdater
	cli := fake.NewSimpleClientset()

	policyInitial := "policy-initial"
	policyUpdated := "policy-updated"

	tmConfInitial := TMConfig{
		Scope:  "scope-initial",
		Policy: "policy-initial",
	}
	tmConfUpdated := TMConfig{
		Scope:  "scope-updated",
		Policy: "polcy-updated",
	}

	var err error
	nrtUpd = NewNRTUpdater(args, policyInitial, tmConfInitial)
	err = nrtUpd.UpdateWithClient(
		cli,
		MonitorInfo{
			Zones: v1alpha1.ZoneList{
				{
					Name: "test-zone-0",
					Type: "node",
					Resources: v1alpha1.ResourceInfoList{
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

	nrtResource := schema.GroupVersionResource{Group: "topology.node.k8s.io", Version: "v1alpha1", Resource: "noderesourcetopologies"}
	obj, err := cli.Tracker().Get(nrtResource, "", nodeName)
	if err != nil {
		t.Fatalf("failed to get the NRT object from tracker: %v", err)
	}
	checkTMPolicy(t, obj, policyInitial, tmConfInitial)

	nrtUpd = NewNRTUpdater(args, policyUpdated, tmConfUpdated)
	err = nrtUpd.UpdateWithClient(
		cli,
		MonitorInfo{
			Zones: v1alpha1.ZoneList{
				{
					Name: "test-zone-0",
					Type: "node",
					Resources: v1alpha1.ResourceInfoList{
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
	checkTMPolicy(t, obj, policyUpdated, tmConfUpdated)
}

func checkTMPolicy(t *testing.T, obj runtime.Object, expectedPolicy string, expectedConf TMConfig) {
	t.Helper()

	nrtObj, ok := obj.(*v1alpha1.NodeResourceTopology)
	if !ok {
		t.Fatalf("provided object is not a NodeResourceTopology")
	}
	if len(nrtObj.TopologyPolicies) != 1 {
		t.Fatalf("unexpected topology policies: %#v", nrtObj.TopologyPolicies)
	}
	if nrtObj.TopologyPolicies[0] != expectedPolicy {
		t.Fatalf("topology policy mismatch: expected %q got %q", expectedPolicy, nrtObj.TopologyPolicies[0])
	}
	zone := findTMConfigZone(nrtObj)
	if zone == nil {
		t.Fatalf("topology manager configuration zone not found")
	}
	gotConf := tmConfigFromAttributes(zone.Attributes)
	if !reflect.DeepEqual(gotConf, expectedConf) {
		t.Fatalf("config got=%+#v expected=%+#v", gotConf, expectedConf)
	}
}

func findTMConfigZone(nodeTopology *v1alpha1.NodeResourceTopology) *v1alpha1.Zone {
	for idx := range nodeTopology.Zones {
		zone := &nodeTopology.Zones[idx]
		if zone.Type != ZoneConfigType {
			continue
		}
		return zone
	}
	return nil
}

func tmConfigFromAttributes(attrs v1alpha1.AttributeList) TMConfig {
	conf := TMConfig{}
	for _, attr := range attrs {
		if attr.Name == "scope" {
			conf.Scope = attr.Value
			continue
		}
		if attr.Name == "policy" {
			conf.Policy = attr.Value
			continue
		}
	}
	return conf
}
