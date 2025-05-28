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

package filter

import (
	podresourcesapi "k8s.io/kubelet/pkg/apis/podresources/v1"
)

type Result struct {
	Allow  bool
	Ident  string // identifier of the object which drove the decision
	Reason string // snakeCase single identifier reason of why the decision
}

func VerifyAlwaysPass(_ *podresourcesapi.PodResources) Result {
	return Result{
		Allow: true,
	}
}

// AlwaysPass is deprecated: use VerifyAlwaysPass instead
func AlwaysPass(pr *podresourcesapi.PodResources) bool {
	ret := VerifyAlwaysPass(pr)
	return ret.Allow
}
