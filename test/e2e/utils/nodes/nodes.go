/*
Copyright 2020 The Kubernetes Authors.

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

package nodes

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	e2etestconsts "github.com/k8stopologyawareschedwg/resource-topology-exporter/test/e2e/utils/testconsts"
)

const (
	// RoleWorker contains the worker role
	RoleWorker = "worker"
)

const (
	// LabelRole contains the key for the role label
	LabelRole = "node-role.kubernetes.io"
)

type patchMapStringStringValue struct {
	Op    string            `json:"op"`
	Path  string            `json:"path"`
	Value map[string]string `json:"value"`
}

// GetWorkerNodes returns all nodes labeled as worker
func GetWorkerNodes(cs kubernetes.Interface) ([]corev1.Node, error) {
	return GetNodesByRole(cs, RoleWorker)
}

// GetNodesByRole GetByRole returns all nodes with the specified role
func GetNodesByRole(cs kubernetes.Interface, role string) ([]corev1.Node, error) {
	selector, err := labels.Parse(fmt.Sprintf("%s/%s=", LabelRole, role))
	if err != nil {
		return nil, err
	}
	return GetNodesBySelector(cs, selector)
}

// GetNodesBySelector GetBySelector returns all nodes with the specified selector
func GetNodesBySelector(cs kubernetes.Interface, selector labels.Selector) ([]corev1.Node, error) {
	nodes, err := cs.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	return nodes.Items, nil
}

// FilterNodesWithEnoughCores returns all nodes with at least the amount of given CPU allocatable
func FilterNodesWithEnoughCores(nodes []corev1.Node, cpuAmount string) ([]corev1.Node, error) {
	requestCpu := resource.MustParse(cpuAmount)
	klog.Infof("checking request %v on %d nodes", requestCpu.String(), len(nodes))

	resNodes := []corev1.Node{}
	for _, node := range nodes {
		availCpu, ok := node.Status.Allocatable[corev1.ResourceCPU]
		if !ok || availCpu.IsZero() {
			return nil, fmt.Errorf("node %q has no allocatable CPU", node.Name)
		}

		if availCpu.Cmp(requestCpu) < 1 {
			klog.Infof("node %q available cpu %v requested cpu %v", node.Name, availCpu.String(), requestCpu.String())
			continue
		}

		klog.Infof("node %q has enough resources, cluster OK", node.Name)
		resNodes = append(resNodes, node)
	}

	return resNodes, nil
}

// LabelNode will add new set of labels to a given node
func LabelNode(cs kubernetes.Interface, node *corev1.Node, newLabels map[string]string) error {
	labelsMap := make(map[string]string)
	labelsMap = node.Labels

	for k, v := range newLabels {
		labelsMap[k] = v
	}

	patchPayload := []patchMapStringStringValue{{
		Op:    "replace",
		Path:  "/metadata/labels",
		Value: labelsMap,
	}}
	payloadBytes, err := json.Marshal(patchPayload)
	if err != nil {
		return err
	}

	_, err = cs.CoreV1().Nodes().Patch(context.TODO(), node.Name, types.JSONPatchType, payloadBytes, metav1.PatchOptions{})
	return err
}

func PickTargetNode(workerNodes []corev1.Node) (*corev1.Node, bool) {
	if len(workerNodes) == 0 {
		return nil, false
	}

	for idx := range workerNodes {
		node := &workerNodes[idx]
		if node.Labels != nil {
			if _, ok := node.Labels[e2etestconsts.TestNodeLabel]; ok {
				return node, true
			}
		}
	}

	return &workerNodes[0], false // any node is fine.
}
