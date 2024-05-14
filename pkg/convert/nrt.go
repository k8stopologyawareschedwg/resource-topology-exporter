/*
Copyright 2024 The Kubernetes Authors.

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

package convert

import (
	k8sresv1alpha2 "k8s.io/api/resource/v1alpha2"

	nrtv1alpha2 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha2"
)

const (
	DriverName = "" // TODO
)

func NodeResourceTopologyToK8SResourceSlice(nrt *nrtv1alpha2.NodeResourceTopology) *k8sresv1alpha2.ResourceSlice {
	res := k8sresv1alpha2.ResourceSlice{
		NodeName:   nrt.Name,
		DriverName: DriverName,
		ResourceModel: k8sresv1alpha2.ResourceModel{
			NamedResources: &k8sresv1alpha2.NamedResourcesResources{
				// nrt.Attributes will be translates to the first Instance
				Instances: make([]k8sresv1alpha2.NamedResourcesInstance, 0, 1+len(nrt.Zones)),
			},
		},
	}
	return &res
}
