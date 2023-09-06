package nrtupdater

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var NotConfigured = errors.New("unconfigured feature")

type NotFound struct {
	NodeName string
}

func (err NotFound) Error() string {
	return "node " + err.NodeName + " Not Found"
}

type ConnectionError struct {
	Err error
}

func (err ConnectionError) Error() string {
	return fmt.Sprintf("error connection k8s: %v", err.Err)
}
func (err ConnectionError) Unwrap() error {
	return err.Err
}

type NodeGetter interface {
	Get(ctx context.Context, nodeName string, opts metav1.GetOptions) (*corev1.Node, error)
}

type DisabledNodeGetter struct {
}

func (ng *DisabledNodeGetter) Get(ctx context.Context, nodeName string, opts metav1.GetOptions) (*corev1.Node, error) {
	return nil, fmt.Errorf("%w", NotConfigured)
}

type CachedNodeGetter struct {
	nodes map[string]*corev1.Node
}

func NewCachedNodeGetter(k8sInterface kubernetes.Interface, ctx context.Context) (*CachedNodeGetter, error) {
	nodelist, err := k8sInterface.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("unable to get node list information: %w", err)
	}

	retVal := &CachedNodeGetter{nodes: make(map[string]*corev1.Node, len(nodelist.Items))}
	for idx := range nodelist.Items {
		node := &nodelist.Items[idx]
		retVal.nodes[node.Name] = node
	}

	return retVal, nil
}

func (ng *CachedNodeGetter) Get(ctx context.Context, nodeName string, _ metav1.GetOptions) (*corev1.Node, error) {
	if node, found := ng.nodes[nodeName]; found {
		return node, nil
	}
	return nil, fmt.Errorf("%w", NotFound{NodeName: nodeName})
}
