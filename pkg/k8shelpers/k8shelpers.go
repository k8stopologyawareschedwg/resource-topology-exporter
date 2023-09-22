package k8shelpers

import (
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	topologyclientset "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/clientset/versioned"
)

func GetTopologyClient(kubeConfig string) (*topologyclientset.Clientset, error) {
	// Set up an in-cluster K8S client.
	var config *restclient.Config
	var err error

	if kubeConfig == "" {
		config, err = restclient.InClusterConfig()
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", kubeConfig)
	}
	if err != nil {
		return nil, err
	}

	topologyClient, err := topologyclientset.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return topologyClient, nil
}

func GetK8sClient(kubeConfig string) (*kubernetes.Clientset, error) {
	// Set up an in-cluster K8S client.
	var config *restclient.Config
	var err error

	if kubeConfig == "" {
		config, err = restclient.InClusterConfig()
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", kubeConfig)
	}
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(config)
}
