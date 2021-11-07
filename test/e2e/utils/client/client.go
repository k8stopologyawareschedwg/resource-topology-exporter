package client

import (
	apiextension "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/kubernetes/test/e2e/framework"
)

func NewK8sExtFromFramework(f *framework.Framework) (*apiextension.Clientset, error) {
	clientset, err := apiextension.NewForConfig(f.ClientConfig())
	if err != nil {
		return nil, err
	}
	return clientset, nil
}
