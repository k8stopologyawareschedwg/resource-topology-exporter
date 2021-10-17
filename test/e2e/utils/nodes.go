package utils

import (
	"context"
	"fmt"
	"os"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/kubernetes/test/e2e/framework"
)

const (
	// RoleWorker contains the worker role
	RoleWorker = "worker"
	// DefaultNodeName we rely on kind for our CI
	DefaultNodeName = "kind-worker"
)

const (
	// LabelRole contains the key for the role label
	LabelRole = "node-role.kubernetes.io"
	// LabelHostname contains the key for the hostname label
	LabelHostname = "kubernetes.io/hostname"
)

// GetWorkerNodes returns all nodes labeled as worker
func GetWorkerNodes(f *framework.Framework) ([]v1.Node, error) {
	return GetNodesByRole(f, RoleWorker)
}

// GetByRole returns all nodes with the specified role
func GetNodesByRole(f *framework.Framework, role string) ([]v1.Node, error) {
	selector, err := labels.Parse(fmt.Sprintf("%s/%s=", LabelRole, role))
	if err != nil {
		return nil, err
	}
	return GetNodesBySelector(f, selector)
}

// GetBySelector returns all nodes with the specified selector
func GetNodesBySelector(f *framework.Framework, selector labels.Selector) ([]v1.Node, error) {
	nodes, err := f.ClientSet.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	return nodes.Items, nil
}

// FilterNodesWithEnoughCores returns all nodes with at least the amount of given CPU allocatable
func FilterNodesWithEnoughCores(nodes []v1.Node, cpuAmount string) ([]v1.Node, error) {
	requestCpu := resource.MustParse(cpuAmount)
	framework.Logf("checking request %v on %d nodes", requestCpu, len(nodes))

	resNodes := []v1.Node{}
	for _, node := range nodes {
		availCpu, ok := node.Status.Allocatable[v1.ResourceCPU]
		if !ok || availCpu.IsZero() {
			return nil, fmt.Errorf("node %q has no allocatable CPU", node.Name)
		}

		if availCpu.Cmp(requestCpu) < 1 {
			framework.Logf("node %q available cpu %v requested cpu %v", node.Name, availCpu, requestCpu)
			continue
		}

		framework.Logf("node %q has enough resources, cluster OK", node.Name)
		resNodes = append(resNodes, node)
	}

	return resNodes, nil
}

func GetNodeName() string {
	if nodeName, ok := os.LookupEnv("E2E_WORKER_NODE_NAME"); ok {
		return nodeName
	}
	return DefaultNodeName
}
