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

package terminalpods

import (
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"
)

func TestFilterFrom(t *testing.T) {
	testCases := []struct {
		filterPods   []*v1.Pod
		resp         *podresourcesapi.ListPodResourcesResponse
		expectedResp *podresourcesapi.ListPodResourcesResponse
	}{
		{
			filterPods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "podA",
						Namespace: "nsA",
					},
				},
			},
			resp: &podresourcesapi.ListPodResourcesResponse{
				PodResources: []*podresourcesapi.PodResources{
					{
						Name:      "podA",
						Namespace: "nsA",
					},
					{
						Name:      "podBB",
						Namespace: "nsA",
					},
					{
						Name:      "podC",
						Namespace: "nsAB",
					},
				},
			},
			expectedResp: &podresourcesapi.ListPodResourcesResponse{
				PodResources: []*podresourcesapi.PodResources{
					{
						Name:      "podBB",
						Namespace: "nsA",
					},
					{
						Name:      "podC",
						Namespace: "nsAB",
					},
				},
			},
		},
		{
			filterPods: []*v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "podA",
						Namespace: "nsA",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: "bar",
					},
				},
			},
			resp: &podresourcesapi.ListPodResourcesResponse{
				PodResources: []*podresourcesapi.PodResources{
					{
						Name:      "foo",
						Namespace: "bar",
					},
					{
						Name:      "bar",
						Namespace: "foo",
					},
					{
						Name:      "podA",
						Namespace: "nsA",
					},
				},
			},
			expectedResp: &podresourcesapi.ListPodResourcesResponse{
				PodResources: []*podresourcesapi.PodResources{
					{
						Name:      "bar",
						Namespace: "foo",
					},
				},
			},
		},
	}

	for i, tc := range testCases {
		FilterFrom(tc.resp, tc.filterPods)
		// sort slices before comparison
		sort.SliceStable(tc.resp.GetPodResources(), func(i, j int) bool {
			return tc.resp.GetPodResources()[i].Name < tc.resp.GetPodResources()[j].Name
		})
		sort.SliceStable(tc.expectedResp.GetPodResources(), func(i, j int) bool {
			return tc.resp.GetPodResources()[i].Name < tc.resp.GetPodResources()[j].Name
		})
		if diff := cmp.Diff(tc.resp, tc.expectedResp); diff != "" {
			t.Errorf("Test%d failed: diff: %s", i, diff)
		}
	}
}
