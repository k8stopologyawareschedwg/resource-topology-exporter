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

	var err error
	nrtUpd = NewNRTUpdater(args, policyInitial)
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
	checkTMPolicy(t, obj, policyInitial)

	nrtUpd = NewNRTUpdater(args, policyUpdated)
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
	checkTMPolicy(t, obj, policyUpdated)
}

func checkTMPolicy(t *testing.T, obj runtime.Object, expectedPolicy string) {
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
}
